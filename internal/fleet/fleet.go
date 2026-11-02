// Package fleet registers the agent with Grafana Fleet Management using OpAMP,
// so that the probe shows up in the Fleet Management collector inventory with
// its attributes and health.
//
// This is a proof of concept: the agent only reports status. It does not
// accept remote configuration, packages or commands from the server.
package fleet

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/open-telemetry/opamp-go/client"
	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

const (
	opampPath    = "/v1/opamp"
	serviceName  = "synthetic-monitoring-agent"
	uidFileName  = "fm-instance-uid"
	stopTimeout  = 5 * time.Second
	maxLocalAttr = 25 // Fleet Management limit on local attributes per collector.
)

var (
	errMissingURL        = errors.New("fleet management URL is required")
	errMissingInstanceID = errors.New("fleet management instance ID is required")
	errMissingToken      = errors.New("fleet management token is required")
)

// Opts configures the Fleet Management client.
type Opts struct {
	Logger zerolog.Logger
	// URL is the Fleet Management base URL. The OpAMP path is appended if
	// it's not already present.
	URL        string
	InstanceID string
	Token      string
	// StateDir is where the OpAMP instance UID is persisted. If empty, a new
	// UID is generated on every start, which makes the probe show up as a new
	// collector each time.
	StateDir string
}

// Agent reports the probe's identity and health to Fleet Management.
type Agent struct {
	logger    zerolog.Logger
	endpoint  string
	header    http.Header
	uid       uuid.UUID
	startTime time.Time

	// fmConnected tracks the connection to Fleet Management, for logging.
	fmConnected atomic.Bool

	mu        sync.Mutex
	client    client.OpAMPClient
	connected bool
	probe     *sm.Probe
}

// New validates the options and loads (or creates) the instance UID.
func New(opts Opts) (*Agent, error) {
	switch {
	case opts.URL == "":
		return nil, errMissingURL
	case opts.InstanceID == "":
		return nil, errMissingInstanceID
	case opts.Token == "":
		return nil, errMissingToken
	}

	endpoint, err := opampEndpoint(opts.URL)
	if err != nil {
		return nil, err
	}

	uid, err := loadOrCreateUID(opts.StateDir)
	if err != nil {
		return nil, err
	}

	if opts.StateDir == "" {
		opts.Logger.Warn().Msg("no fleet management state directory configured; the probe will register as a new collector on every restart")
	}

	auth := base64.StdEncoding.EncodeToString([]byte(opts.InstanceID + ":" + opts.Token))

	return &Agent{
		logger:    opts.Logger,
		endpoint:  endpoint,
		header:    http.Header{"Authorization": []string{"Basic " + auth}},
		uid:       uid,
		startTime: time.Now(),
	}, nil
}

// Run starts the OpAMP client and blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	c := client.NewHTTP(&opampLogger{logger: a.logger})

	a.mu.Lock()
	a.client = c

	if err := c.SetAgentDescription(a.description()); err != nil {
		a.mu.Unlock()
		return fmt.Errorf("setting agent description: %w", err)
	}

	capabilities := protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus |
		protobufs.AgentCapabilities_AgentCapabilities_ReportsHealth |
		protobufs.AgentCapabilities_AgentCapabilities_ReportsHeartbeat
	if err := c.SetCapabilities(&capabilities); err != nil {
		a.mu.Unlock()
		return fmt.Errorf("setting agent capabilities: %w", err)
	}

	if err := c.SetHealth(a.health()); err != nil {
		a.mu.Unlock()
		return fmt.Errorf("setting agent health: %w", err)
	}
	a.mu.Unlock()

	err := c.Start(ctx, types.StartSettings{
		OpAMPServerURL: a.endpoint,
		Header:         a.header,
		InstanceUid:    types.InstanceUid(a.uid),
		Callbacks: types.Callbacks{
			// The HTTP transport calls OnConnect on every successful poll,
			// so only log transitions.
			OnConnect: func(context.Context) {
				if !a.fmConnected.Swap(true) {
					a.logger.Info().Msg("connected to fleet management")
				}
			},
			OnConnectFailed: func(_ context.Context, err error) {
				a.fmConnected.Store(false)
				a.logger.Warn().Err(err).Msg("failed to connect to fleet management")
			},
			OnError: func(_ context.Context, resp *protobufs.ServerErrorResponse) {
				a.logger.Warn().Str("error", resp.GetErrorMessage()).Msg("fleet management returned an error")
			},
		},
	})
	if err != nil {
		return fmt.Errorf("starting fleet management client: %w", err)
	}

	a.logger.Info().Str("endpoint", a.endpoint).Str("instance_uid", a.uid.String()).Msg("fleet management client started")

	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()

	return c.Stop(stopCtx)
}

// SetConnected records whether the agent is currently connected to the
// Synthetic Monitoring API, which is what the probe's health reflects.
func (a *Agent) SetConnected(connected bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.connected == connected {
		return
	}

	a.connected = connected

	if a.client != nil {
		if err := a.client.SetHealth(a.health()); err != nil {
			a.logger.Warn().Err(err).Msg("failed to update fleet management health")
		}
	}
}

// SetProbe adds the probe's details, as returned by the Synthetic Monitoring
// API on registration, to the attributes reported to Fleet Management.
func (a *Agent) SetProbe(probe *sm.Probe) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.probe = probe

	if a.client != nil {
		if err := a.client.SetAgentDescription(a.description()); err != nil {
			a.logger.Warn().Err(err).Msg("failed to update fleet management agent description")
		}
	}
}

// description must be called with a.mu held.
func (a *Agent) description() *protobufs.AgentDescription {
	identifying := []*protobufs.KeyValue{
		stringKV("service.name", serviceName),
		stringKV("service.version", version.Short()),
		stringKV("service.instance.id", a.uid.String()),
	}

	nonIdentifying := []*protobufs.KeyValue{
		stringKV("os.type", runtime.GOOS),
		stringKV("host.arch", runtime.GOARCH),
	}

	if hostname, err := os.Hostname(); err == nil {
		nonIdentifying = append(nonIdentifying, stringKV("host.name", hostname))
	}

	if a.probe != nil {
		nonIdentifying = append(nonIdentifying,
			stringKV("sm.probe.id", strconv.FormatInt(a.probe.Id, 10)),
			stringKV("sm.probe.name", a.probe.Name),
			stringKV("sm.probe.region", a.probe.Region),
		)

		labels := make([]sm.Label, len(a.probe.Labels))
		copy(labels, a.probe.Labels)
		sort.Slice(labels, func(i, j int) bool { return labels[i].Name < labels[j].Name })

		for _, l := range labels {
			if len(identifying)+len(nonIdentifying) >= maxLocalAttr {
				break
			}

			nonIdentifying = append(nonIdentifying, stringKV("sm.probe.label."+l.Name, l.Value))
		}
	}

	return &protobufs.AgentDescription{
		IdentifyingAttributes:    identifying,
		NonIdentifyingAttributes: nonIdentifying,
	}
}

// health must be called with a.mu held.
func (a *Agent) health() *protobufs.ComponentHealth {
	h := &protobufs.ComponentHealth{
		Healthy:            a.connected,
		StartTimeUnixNano:  uint64(a.startTime.UnixNano()),
		StatusTimeUnixNano: uint64(time.Now().UnixNano()),
		Status:             "connected to Synthetic Monitoring API",
	}

	if !a.connected {
		h.Status = "not connected to Synthetic Monitoring API"
		h.LastError = h.Status
	}

	return h
}

func opampEndpoint(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parsing fleet management URL: %w", err)
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("fleet management URL must be http or https, got %q", u.Scheme)
	}

	u.Path = strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(u.Path, opampPath) {
		u.Path += opampPath
	}

	return u.String(), nil
}

func loadOrCreateUID(stateDir string) (uuid.UUID, error) {
	if stateDir == "" {
		return uuid.NewV7()
	}

	path := filepath.Join(stateDir, uidFileName)

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		uid, err := uuid.ParseBytes([]byte(strings.TrimSpace(string(data))))
		if err != nil {
			return uuid.UUID{}, fmt.Errorf("parsing instance UID from %s: %w", path, err)
		}

		return uid, nil

	case !errors.Is(err, os.ErrNotExist):
		return uuid.UUID{}, fmt.Errorf("reading instance UID: %w", err)
	}

	uid, err := uuid.NewV7()
	if err != nil {
		return uuid.UUID{}, err
	}

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return uuid.UUID{}, fmt.Errorf("creating fleet management state directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(uid.String()+"\n"), 0o600); err != nil {
		return uuid.UUID{}, fmt.Errorf("writing instance UID: %w", err)
	}

	return uid, nil
}

func stringKV(key, value string) *protobufs.KeyValue {
	return &protobufs.KeyValue{
		Key:   key,
		Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: value}},
	}
}

// opampLogger adapts zerolog to the opamp-go client logger interface.
type opampLogger struct {
	logger zerolog.Logger
}

func (l *opampLogger) Debugf(_ context.Context, format string, v ...any) {
	l.logger.Debug().Msgf(format, v...)
}

func (l *opampLogger) Errorf(_ context.Context, format string, v ...any) {
	l.logger.Error().Msgf(format, v...)
}
