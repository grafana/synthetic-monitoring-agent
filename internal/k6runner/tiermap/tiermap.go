// Package tiermap routes a (check type, tenant ID) pair to a worker tier name.
//
// The mapping is loaded from a YAML document with the shape:
//
//	browser:
//	  default: browser-A
//	  tenants:
//	    "1234": browser-B
//	small:
//	  default: small
//
// The mapping reload path is intentionally split from the file IO: callers
// build a [Mapper] with [New] (which validates the bytes) and swap into a
// live [Live] via [Live.Reload] when they detect a change on disk. The
// validation/swap split lets the dispatcher keep serving with the
// last-known-good mapping when a write produces malformed YAML.
package tiermap

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

// Family names. The check type alone determines the family; the tenant
// mapping selects the tier within the family.
const (
	FamilySmall   = "small"
	FamilyBrowser = "browser"
)

// Errors returned by the package.
var (
	// ErrUnknownCheckType is returned when [Mapper.Tier] is called with a
	// check type that does not map to any known family.
	ErrUnknownCheckType = errors.New("unknown check type")
	// ErrUnknownTier is returned when the resolved tier name does not
	// exist in the loaded mapping. Callers should refuse to dispatch and
	// surface this via the tenant_tier_mapping_errors_total metric.
	ErrUnknownTier = errors.New("unknown tier")
)

// FamilyForCheckType returns the tier family for a given SM check type
// string (as produced by [github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring.CheckType.String]).
// scripted and multihttp share the small family; browser is its own
// family.
func FamilyForCheckType(checkType string) (string, error) {
	switch checkType {
	case "scripted", "multihttp":
		return FamilySmall, nil
	case "browser":
		return FamilyBrowser, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownCheckType, checkType)
	}
}

// File is the on-disk shape of the mapping document. Each family has a
// default tier (used when a tenant is not listed) and an optional table
// of tenant overrides keyed by tenant ID expressed as a string.
type File struct {
	Browser FamilyConfig `yaml:"browser"`
	Small   FamilyConfig `yaml:"small"`
}

// FamilyConfig holds the per-family default tier and per-tenant
// overrides.
type FamilyConfig struct {
	Default string            `yaml:"default"`
	Tenants map[string]string `yaml:"tenants"`
}

// Mapper is an immutable, parsed mapping. It is safe to read concurrently
// without locking. Use [Live] when the mapping needs to be reloaded
// without restarting the process.
type Mapper struct {
	browserDefault string
	browserTenants map[string]string
	smallDefault   string
	smallTenants   map[string]string
	// knownTiers records the tier names referenced by the mapping. The
	// dispatcher uses this to detect mappings that point at tiers no
	// worker pool serves.
	knownTiers map[string]struct{}
}

// New parses YAML bytes and returns a [Mapper]. It returns an error if
// the YAML cannot be parsed or if a required default is missing.
func New(b []byte) (*Mapper, error) {
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parsing tier-mapping YAML: %w", err)
	}

	if f.Browser.Default == "" {
		return nil, errors.New("missing browser.default tier")
	}
	if f.Small.Default == "" {
		return nil, errors.New("missing small.default tier")
	}

	known := map[string]struct{}{
		f.Browser.Default: {},
		f.Small.Default:   {},
	}
	for _, t := range f.Browser.Tenants {
		known[t] = struct{}{}
	}
	for _, t := range f.Small.Tenants {
		known[t] = struct{}{}
	}

	return &Mapper{
		browserDefault: f.Browser.Default,
		browserTenants: copyStringMap(f.Browser.Tenants),
		smallDefault:   f.Small.Default,
		smallTenants:   copyStringMap(f.Small.Tenants),
		knownTiers:     known,
	}, nil
}

// Tier resolves the tier name for a (checkType, tenantID) pair. It
// returns the tier and the family that selected it. ErrUnknownCheckType
// is returned for unrecognised check types.
func (m *Mapper) Tier(checkType, tenantID string) (tier, family string, err error) {
	family, err = FamilyForCheckType(checkType)
	if err != nil {
		return "", "", err
	}

	switch family {
	case FamilyBrowser:
		if t, ok := m.browserTenants[tenantID]; ok {
			return t, family, nil
		}
		return m.browserDefault, family, nil
	case FamilySmall:
		if t, ok := m.smallTenants[tenantID]; ok {
			return t, family, nil
		}
		return m.smallDefault, family, nil
	default:
		// FamilyForCheckType is the only producer of family; this case is unreachable.
		return "", "", fmt.Errorf("%w: %q", ErrUnknownCheckType, checkType)
	}
}

// KnownTiers returns the set of tier names referenced by the mapping
// (as defaults or per-tenant overrides). Callers that know the deployed
// worker pools can subtract this set from the deployed set to detect
// mapping entries that point at undeployed tiers.
func (m *Mapper) KnownTiers() []string {
	out := make([]string, 0, len(m.knownTiers))
	for t := range m.knownTiers {
		out = append(out, t)
	}
	return out
}

// Live wraps a [Mapper] behind atomic-swap semantics. Reads return the
// current mapping; [Live.Reload] swaps in a new one without invalidating
// any reader's reference. On parse error the previous mapping is kept.
type Live struct {
	current atomic.Pointer[Mapper]
	metrics *Metrics
	logger  zerolog.Logger
}

// NewLive constructs a [Live] from an initial mapping. The initial
// mapping must succeed; this is a startup-time precondition.
func NewLive(initial *Mapper, metrics *Metrics, logger zerolog.Logger) *Live {
	l := &Live{metrics: metrics, logger: logger}
	l.current.Store(initial)
	return l
}

// Current returns the live mapping. Safe for concurrent use; the
// returned [Mapper] is immutable.
func (l *Live) Current() *Mapper {
	return l.current.Load()
}

// Reload parses the supplied YAML and swaps in the new mapping. On
// parse error the live mapping is unchanged and an error is returned;
// the parse_error counter is incremented.
func (l *Live) Reload(b []byte) error {
	m, err := New(b)
	if err != nil {
		if l.metrics != nil {
			l.metrics.MappingErrors.WithLabelValues("parse_error").Inc()
		}
		l.logger.Error().Err(err).Msg("tier mapping reload failed; keeping last-known-good")
		return err
	}
	l.current.Store(m)
	l.logger.Info().Strs("knownTiers", m.KnownTiers()).Msg("tier mapping reloaded")
	return nil
}

// Tier is a convenience wrapper around [Live.Current] + [Mapper.Tier].
func (l *Live) Tier(checkType, tenantID string) (tier, family string, err error) {
	return l.Current().Tier(checkType, tenantID)
}

// Watch polls path for modifications at the given interval and calls
// [Live.Reload] when the file's modification time changes. Watch returns
// when ctx is cancelled. The interval should be set well below kubelet's
// ConfigMap propagation period (~60s) so changes land within an SLO of a
// few minutes; 30s is a reasonable default.
func (l *Live) Watch(ctx context.Context, path string, interval time.Duration) {
	if interval <= 0 {
		panic("tiermap: Watch called with non-positive interval")
	}

	var lastMtime time.Time
	if info, err := os.Stat(path); err == nil {
		lastMtime = info.ModTime()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(path)
			if err != nil {
				l.logger.Error().Err(err).Str("path", path).Msg("stat tier mapping path")
				continue
			}
			if !info.ModTime().After(lastMtime) {
				continue
			}
			b, err := os.ReadFile(path) //nolint:gosec // path is operator-supplied and trusted.
			if err != nil {
				l.logger.Error().Err(err).Str("path", path).Msg("read tier mapping path")
				continue
			}
			if reloadErr := l.Reload(b); reloadErr == nil {
				lastMtime = info.ModTime()
			}
			// On reload error keep lastMtime unchanged so a retry happens on the next tick if the file is updated.
		}
	}
}

// Metrics holds the prometheus collectors used by [Live] and the
// dispatcher when reporting mapping problems.
type Metrics struct {
	// MappingErrors counts mapping problems by cause:
	//
	//   - parse_error: the YAML failed to parse on reload.
	//   - unknown_tier: a tenant entry resolved to a tier that no worker
	//     pool serves (incremented by the dispatcher, not by the
	//     package itself, since the package does not know which tiers
	//     are deployed).
	MappingErrors *prometheus.CounterVec
}

// NewMetrics registers the mapping metrics on the supplied registerer.
func NewMetrics(r prometheus.Registerer) *Metrics {
	m := &Metrics{
		MappingErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "k6_runner",
				Subsystem: "tier_mapping",
				Name:      "errors_total",
				Help: "Number of tier-mapping problems by cause. " +
					"parse_error: a reload was attempted but the YAML did not parse and the live mapping was kept. " +
					"unknown_tier: a tenant entry resolved to a tier with no deployed worker pool.",
			},
			[]string{"cause"},
		),
	}
	r.MustRegister(m.MappingErrors)
	return m
}

// Cause label values for [Metrics.MappingErrors].
const (
	CauseParseError  = "parse_error"
	CauseUnknownTier = "unknown_tier"
)

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}
