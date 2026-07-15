package backfill

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	logproto "github.com/grafana/loki/pkg/push"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/scraper"
	"github.com/grafana/synthetic-monitoring-agent/internal/telemetry"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
)

type TimeSeries = []prompb.TimeSeries
type Streams = []logproto.Stream

// NewGeneratorFromSM builds a generator from API-level check and probe messages.
func NewGeneratorFromSM(ctx context.Context, check sm.Check, probe sm.Probe) (*Generator, error) {
	var modelCheck model.Check
	if err := modelCheck.FromSM(check); err != nil {
		return nil, err
	}
	return NewGenerator(ctx, modelCheck, probe)
}

// Generator produces agent-faithful metrics and logs for synthetic executions.
type Generator struct {
	scraper *scraper.Scraper
	prober  *SyntheticHTTPProber // retained for Collect's byte-identical HTTP path
	typed   SyntheticProber      // used by CollectTyped; set for every check type
}

// NewGenerator wires a scraper with a synthetic HTTP prober for the given check and probe.
func NewGenerator(ctx context.Context, check model.Check, probe sm.Probe) (*Generator, error) {
	synthetic := NewSyntheticHTTPProber(check.Target)
	factory := syntheticProbeFactory{prober: synthetic}

	s, err := scraper.NewWithOpts(ctx, check, scraper.ScraperOpts{
		Probe:                 probe,
		Publisher:             noopPublisher{},
		Logger:                zerolog.New(io.Discard),
		Metrics:               noopMetrics{},
		ProbeFactory:          factory,
		LabelsLimiter:         noopLabelsLimiter{},
		Telemeter:             noopTelemeter{},
		CostAttributionLabels: noopTenantCals{},
	})
	if err != nil {
		return nil, err
	}

	return &Generator{scraper: s, prober: synthetic, typed: synthetic}, nil
}

// Collect runs one synthetic execution at timestamp t.
func (g *Generator) Collect(ctx context.Context, t time.Time, sample Sample) (TimeSeries, Streams, error) {
	sample.At = t.UTC()
	sample.Normalize()
	g.prober.SetSample(sample)

	ts, streams, _, _, err := g.scraper.CollectData(ctx, t.UTC())
	return ts, streams, err
}

// proberConstructors is the explicit, reflection-free registry of synthetic
// prober constructors keyed by sm.CheckType. NewGeneratorForCheck dispatches
// through this map. Register each new check type here as it's implemented
// (A2-A6): one constructor entry per type, e.g.
//
//	sm.CheckTypePing: func(target string) SyntheticProber {
//	    return NewSyntheticPingProber(target)
//	},
var proberConstructors = map[sm.CheckType]func(target string) SyntheticProber{
	sm.CheckTypeHttp: func(target string) SyntheticProber {
		return NewSyntheticHTTPProber(target)
	},
	sm.CheckTypeDns: func(target string) SyntheticProber {
		return NewSyntheticDNSProber(target)
	},
	sm.CheckTypeTcp: func(target string) SyntheticProber {
		return NewSyntheticTCPProber(target)
	},
	sm.CheckTypePing: func(target string) SyntheticProber {
		return NewSyntheticPingProber(target)
	},
	sm.CheckTypeTraceroute: func(target string) SyntheticProber {
		return NewSyntheticTracerouteProber(target)
	},
	sm.CheckTypeGrpc: func(target string) SyntheticProber {
		return NewSyntheticGRPCProber(target)
	},
}

// NewGeneratorForCheck builds a generator whose synthetic prober is selected
// by the check's type via proberConstructors. Only sm.CheckTypeHttp is
// registered so far; other types (including scripted, multihttp, and
// browser) return an error naming the currently supported types.
func NewGeneratorForCheck(ctx context.Context, check sm.Check, probe sm.Probe) (*Generator, error) {
	var modelCheck model.Check
	if err := modelCheck.FromSM(check); err != nil {
		return nil, err
	}

	if !hasCheckSettings(check) {
		return nil, errors.New("backfill: check has no settings; cannot determine type")
	}

	checkType := check.Type()
	constructor, ok := proberConstructors[checkType]
	if !ok {
		return nil, fmt.Errorf("backfill: unsupported check type %q, supported types: %s", checkType, supportedCheckTypes())
	}

	synthetic := constructor(modelCheck.Target)
	factory := syntheticProberFactory{prober: synthetic}

	s, err := scraper.NewWithOpts(ctx, modelCheck, scraper.ScraperOpts{
		Probe:                 probe,
		Publisher:             noopPublisher{},
		Logger:                zerolog.New(io.Discard),
		Metrics:               noopMetrics{},
		ProbeFactory:          factory,
		LabelsLimiter:         noopLabelsLimiter{},
		Telemeter:             noopTelemeter{},
		CostAttributionLabels: noopTenantCals{},
	})
	if err != nil {
		return nil, err
	}

	gen := &Generator{scraper: s, typed: synthetic}
	if httpProber, ok := synthetic.(*SyntheticHTTPProber); ok {
		gen.prober = httpProber
	}
	return gen, nil
}

// CollectTyped runs one synthetic execution at timestamp t using the
// check-type-agnostic TypedSample/SyntheticProber path built by
// NewGeneratorForCheck.
func (g *Generator) CollectTyped(ctx context.Context, t time.Time, s TypedSample) (TimeSeries, Streams, error) {
	s = s.WithTimestamp(t.UTC()).Normalize()
	g.typed.SetTyped(s)

	ts, streams, _, _, err := g.scraper.CollectData(ctx, t.UTC())
	return ts, streams, err
}

// hasCheckSettings reports whether check.Settings has any sub-message set.
// sm.Check.Type() panics ("unhandled check type") when none are, so callers
// that dispatch on Type() must guard with this first.
func hasCheckSettings(check sm.Check) bool {
	s := check.Settings
	return s.Dns != nil ||
		s.Http != nil ||
		s.Ping != nil ||
		s.Tcp != nil ||
		s.Traceroute != nil ||
		s.Scripted != nil ||
		s.Multihttp != nil ||
		s.Grpc != nil ||
		s.Browser != nil
}

func supportedCheckTypes() string {
	types := make([]string, 0, len(proberConstructors))
	for t := range proberConstructors {
		types = append(types, t.String())
	}
	sort.Strings(types)
	return strings.Join(types, ", ")
}

type syntheticProbeFactory struct {
	prober *SyntheticHTTPProber
}

func (f syntheticProbeFactory) New(ctx context.Context, logger zerolog.Logger, check model.Check) (prober.Prober, string, error) {
	return f.prober, check.Target, nil
}

// syntheticProberFactory is the check-type-agnostic counterpart of
// syntheticProbeFactory, used by NewGeneratorForCheck for every registered
// SyntheticProber (HTTP included).
type syntheticProberFactory struct {
	prober SyntheticProber
}

func (f syntheticProberFactory) New(ctx context.Context, logger zerolog.Logger, check model.Check) (prober.Prober, string, error) {
	return f.prober, check.Target, nil
}

type noopPublisher struct{}

func (noopPublisher) Publish(pusher.Payload) {}

type noopMetrics struct{}

func (noopMetrics) AddScrape()         {}
func (noopMetrics) AddCheckError()     {}
func (noopMetrics) AddCollectorError() {}

type noopLabelsLimiter struct{}

func (noopLabelsLimiter) MetricLabels(context.Context, model.GlobalID) (int, error) {
	return 128, nil
}

func (noopLabelsLimiter) LogLabels(context.Context, model.GlobalID) (int, error) {
	return 128, nil
}

type noopTelemeter struct{}

func (noopTelemeter) AddExecution(telemetry.Execution) {}

type noopTenantCals struct{}

func (noopTenantCals) CostAttributionLabels(context.Context, model.GlobalID) ([]string, error) {
	return nil, nil
}
