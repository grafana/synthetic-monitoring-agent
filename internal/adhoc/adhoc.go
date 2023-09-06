package adhoc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/logproto"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// Handler is in charge of retrieving ad-hoc checks from the
// synthetic-monitoring-api, running them and forwarding the results to
// the publisher.
type Handler struct {
	api                          apiInfo
	logger                       zerolog.Logger
	features                     feature.Collection
	backoff                      Backoffer
	probe                        *sm.Probe
	metrics                      metrics
	publisher                    pusher.Publisher
	tenantCh                     chan<- sm.Tenant
	runnerFactory                func(context.Context, *sm.AdHocRequest) (*runner, error)
	grpcAdhocChecksClientFactory func(conn ClientConn) (sm.AdHocChecksClient, error)
	proberFactory                prober.ProberFactory
}

// Error represents errors returned from this package.
type Error string

func (e Error) Error() string { return string(e) }

const (
	errNotAuthorized     = Error("probe not authorized")
	errTransportClosing  = Error("transport closing")
	errProbeUnregistered = Error("probe no longer registered")
	errIncompatibleApi   = Error("API does not support required features")
)

type runner struct {
	logger  zerolog.Logger
	prober  prober.Prober
	id      string
	target  string
	probe   string
	timeout time.Duration
}

// ClientConn represents the GRPC client connection that can be used to
// make RPC calls to the Synthetic Monitoring API.
type ClientConn interface {
	grpc.ClientConnInterface
	GetState() connectivity.State
}

type apiInfo struct {
	conn ClientConn
}

type metrics struct {
	opsCounter *prometheus.CounterVec
}

// Backoffer defines an interface to provide backoff durations.
//
// The implementation of this interface SHOULD NOT perform the actual
// sleep, but rather return the duration to sleep.
type Backoffer interface {
	// Reset causes the backoff provider to go back its initial
	// state, before any calls to Duration() were made.
	Reset()
	// Duration returns the duration to sleep.
	Duration() time.Duration
}

type constantBackoff time.Duration

func (constantBackoff) Reset() {}

func (b constantBackoff) Duration() time.Duration { return time.Duration(b) }

// HandlerOpts is used to pass configuration options to the Handler.
type HandlerOpts struct {
	Conn           ClientConn
	Logger         zerolog.Logger
	Backoff        Backoffer
	Publisher      pusher.Publisher
	TenantCh       chan<- sm.Tenant
	PromRegisterer prometheus.Registerer
	Features       feature.Collection
	K6Runner       k6runner.Runner

	// these two fields exists so that tests can pass alternate
	// implementations, they are unexported so that clients of this
	// package always use the default ones.
	runnerFactory                func(context.Context, *sm.AdHocRequest) (*runner, error)
	grpcAdhocChecksClientFactory func(conn ClientConn) (sm.AdHocChecksClient, error)
}

// NewHandler creates a new Handler using the specified options.
func NewHandler(opts HandlerOpts) (*Handler, error) {
	// We should never hit this, but just in case.
	if !opts.Features.IsSet(feature.AdHoc) {
		return nil, fmt.Errorf("AdHoc feature is not enabled")
	}

	opsCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm",
			Subsystem: "adhoc",
			Name:      "ops_total",
			Help:      "Total number of adhoc operations",
		},
		[]string{"type"},
	)

	if err := opts.PromRegisterer.Register(opsCounter); err != nil {
		return nil, err
	}

	h := &Handler{
		logger:                       opts.Logger,
		features:                     opts.Features,
		backoff:                      opts.Backoff,
		publisher:                    opts.Publisher,
		tenantCh:                     opts.TenantCh,
		runnerFactory:                opts.runnerFactory,
		grpcAdhocChecksClientFactory: opts.grpcAdhocChecksClientFactory,
		proberFactory:                prober.NewProberFactory(opts.K6Runner),
		api: apiInfo{
			conn: opts.Conn,
		},
		metrics: metrics{
			opsCounter: opsCounter,
		},
	}

	if opts.runnerFactory == nil {
		h.runnerFactory = h.defaultRunnerFactory
	}

	if opts.grpcAdhocChecksClientFactory == nil {
		h.grpcAdhocChecksClientFactory = defaultGrpcAdhocChecksClientFactory
	}

	if opts.Backoff == nil {
		h.backoff = constantBackoff(60 * time.Second)
	}

	return h, nil
}

// Run starts the handler.
func (h *Handler) Run(ctx context.Context) error {
	for {
		err := h.loop(ctx)
		switch {
		case err == nil:
			return nil

		case errors.Is(err, errNotAuthorized):
			// our token is invalid, bail out?
			h.logger.Error().
				Err(err).
				Str("connection_state", h.api.conn.GetState().String()).
				Msg("cannot connect, bailing out")
			return err

		case errors.Is(err, errIncompatibleApi):
			// API server doesn't support required features.
			h.logger.Error().
				Err(err).
				Str("connection_state", h.api.conn.GetState().String()).
				Msg("cannot connect, bailing out")
			return err

		case errors.Is(err, errTransportClosing):
			// the other end went away? Allow GRPC to reconnect.
			h.logger.Warn().
				Err(err).
				Str("connection_state", h.api.conn.GetState().String()).
				Msg("the other end closed the connection, trying to reconnect")

			// After disconnecting, reset the backoff
			// counter to start afresh.
			h.backoff.Reset()

			continue

		case errors.Is(err, context.Canceled):
			// context was cancelled, clean up
			h.logger.Error().
				Err(err).
				Str("connection_state", h.api.conn.GetState().String()).
				Msg("context cancelled, closing updater")
			return nil

		case errors.Is(err, errProbeUnregistered):
			// Probe unregistered itself from the API, wait
			// until attempting to reconnect again.
			h.logger.Warn().
				Str("connection_state", h.api.conn.GetState().String()).
				Msg("unregistered probe in API, sleeping for 1 minute...")

			if err := sleepCtx(ctx, 1*time.Minute); err != nil {
				return err
			}

			// The probe is going to reconnect, reset the
			// backoff counter to start afresh.
			h.backoff.Reset()

		default:
			h.logger.Warn().
				Err(err).
				Str("connection_state", h.api.conn.GetState().String()).
				Msg("handling check changes")

			// TODO(mem): this might be a transient error (e.g. bad connection). We probably need to
			// fine-tune GRPPC's backoff parameters. We might also need to keep count of the reconnects, and
			// give up if they hit some threshold?
			if err := sleepCtx(ctx, h.backoff.Duration()); err != nil {
				return err
			}
		}
	}
}

func (h *Handler) loop(ctx context.Context) error {
	h.logger.Info().Msg("fetching ad-hoc checks from synthetic-monitoring-api")

	client, err := h.grpcAdhocChecksClientFactory(h.api.conn)
	if err != nil {
		return fmt.Errorf("cannot create adhoc checks client: %w", err)
	}

	grpcErrorHandler := func(action string, err error) error {
		status, ok := status.FromError(err)
		h.logger.Error().Err(err).Bool("ok", ok).Str("action", action).Uint32("code", uint32(status.Code())).Msg(status.Message())

		switch {
		case !ok:
			return fmt.Errorf("%s: %w", action, err)

		case status.Code() == codes.Canceled:
			// either we were told to shut down
			return context.Canceled

		case status.Message() == "transport is closing":
			// the other end is shutting down
			return errTransportClosing

		case status.Code() == codes.PermissionDenied:
			return errNotAuthorized

		case status.Code() == codes.Unimplemented:
			return errIncompatibleApi

		default:
			return status.Err()
		}
	}

	result, err := client.RegisterProbe(
		ctx,
		&sm.ProbeInfo{
			Version:    version.Short(),
			Commit:     version.Commit(),
			Buildstamp: version.Buildstamp(),
		},
	)
	if err != nil {
		return grpcErrorHandler(
			"registering ad-hoc probe with synthetic-monitoring-api",
			err)
	}

	switch result.Status.Code {
	case sm.StatusCode_OK:
		// continue

	case sm.StatusCode_NOT_AUTHORIZED:
		return errNotAuthorized

	default:
		return fmt.Errorf(
			"registering ad-hoc probe with synthetic-monitoring-api, response: %s",
			result.Status.Message)
	}

	h.probe = &result.Probe

	h.logger.Info().
		Int64("probe id", h.probe.Id).
		Str("probe name", h.probe.Name).
		Msg("registered ad-hoc probe with synthetic-monitoring-api")

	requests, err := client.GetAdHocChecks(ctx, &sm.Void{})
	if err != nil {
		return grpcErrorHandler("requesting ad-hoc checks from synthetic-monitoring-api", err)
	}

	return h.processAdHocChecks(ctx, requests)
}

func (h *Handler) processAdHocChecks(ctx context.Context, client sm.AdHocChecks_GetAdHocChecksClient) error {
	for {
		select {
		case <-client.Context().Done():
			return nil

		case <-ctx.Done():
			return ctx.Err()

		default:
			switch msg, err := client.Recv(); err {
			case nil:
				if err := h.handleAdHocCheck(ctx, msg); err != nil {
					h.logger.Error().Err(err).Interface("request", msg).Msg("handling ad-hoc check")
					continue
				}

			case io.EOF:
				h.logger.Warn().Err(err).Msg("no more messages?")
				// XXX(mem): what happened here? The
				// other end told us there are no more
				// changes. Stop? Is it restarting?
				return nil

			default:
				h.logger.Error().Err(err).Msg("receiving ad-hoc check")
				return err
			}
		}
	}
}

func (h *Handler) handleAdHocCheck(ctx context.Context, ahReq *sm.AdHocRequest) error {
	h.logger.Debug().Interface("request", ahReq).Msg("got ad-hoc check request")

	h.metrics.opsCounter.WithLabelValues(ahReq.AdHocCheck.Type().String()).Inc()

	runner, err := h.runnerFactory(ctx, ahReq)
	if err != nil {
		return err
	}

	go runner.Run(ctx, model.GlobalID(ahReq.AdHocCheck.TenantId), h.publisher)

	// If there's a tenant in the request, this should be forwarded
	// to the changes handler.
	if ahReq.Tenant != nil {
		h.tenantCh <- *ahReq.Tenant
	}

	return nil
}

func defaultGrpcAdhocChecksClientFactory(conn ClientConn) (sm.AdHocChecksClient, error) {
	cc, ok := conn.(*grpc.ClientConn)
	if !ok {
		return nil, fmt.Errorf("unexpected type of connection: %T", conn)
	}

	return sm.NewAdHocChecksClient(cc), nil
}

func (h *Handler) defaultRunnerFactory(ctx context.Context, req *sm.AdHocRequest) (*runner, error) {
	check := model.Check{
		Check: sm.Check{
			Target:   req.AdHocCheck.Target,
			Timeout:  req.AdHocCheck.Timeout,
			Settings: req.AdHocCheck.Settings,
		},
	}

	p, target, err := h.proberFactory.New(ctx, h.logger, check)
	if err != nil {
		return nil, err
	}

	return &runner{
		logger:  h.logger,
		prober:  p,
		id:      req.AdHocCheck.Id,
		target:  target,
		probe:   h.probe.Name,
		timeout: time.Duration(req.AdHocCheck.Timeout) * time.Millisecond,
	}, nil
}

// sleepCtx is like time.Sleep, but it pays attention to the
// cancellation of the provided context.
func sleepCtx(ctx context.Context, d time.Duration) error {
	var err error

	timer := time.NewTimer(d)

	select {
	case <-ctx.Done():
		err = ctx.Err()

		if !timer.Stop() {
			<-timer.C
		}

	case <-timer.C:
	}

	return err
}

// jsonLogger implements the log.Logger interface.
type jsonLogger struct {
	entries []map[string]string
}

// Log takes key-value pairs and logs them.
func (l *jsonLogger) Log(keyvals ...interface{}) error {
	m := make(map[string]string)
	if len(keyvals)%2 != 0 {
		return fmt.Errorf("expected even number of keyvals, got %d", len(keyvals))
	}
	for i := 0; i < len(keyvals); i += 2 {
		k := fmt.Sprintf("%v", keyvals[i])
		v := fmt.Sprintf("%v", keyvals[i+1])
		m[k] = v
	}
	l.entries = append(l.entries, m)
	return nil
}

// Run runs the specified prober once and captures the results using
// jsonLogger.
func (r *runner) Run(ctx context.Context, tenantId model.GlobalID, publisher pusher.Publisher) {
	r.logger.Info().Msg("running ad-hoc check")

	registry := prometheus.NewRegistry()

	logger := &jsonLogger{}

	// TODO(mem): decide what to do with these metrics.
	successGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_success",
		Help: "whether the check was successful",
	})

	registry.MustRegister(successGauge)

	durationGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_duration_seconds",
		Help: "duration of the check in seconds",
	})

	registry.MustRegister(durationGauge)

	start := time.Now()

	rCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	if r.prober.Probe(rCtx, r.target, registry, logger) {
		successGauge.Set(1)
	} else {
		successGauge.Set(0)
	}

	durationGauge.Set(float64(time.Since(start).Microseconds()) / 1e6)

	mfs, err := registry.Gather()

	buf := &bytes.Buffer{}
	targetLogger := zerolog.New(buf)

	targetLogger.Info().
		AnErr("error", err).
		Str("id", r.id).
		Str("target", r.target).
		Str("probe", r.probe).
		Str("check_name", r.prober.Name()).
		Interface("logs", logger.entries).
		Interface("timeseries", mfs).
		Msg("ad-hoc check done")

	r.logger.Debug().
		Str("id", r.id).
		Str("target", r.target).
		Str("probe", r.probe).
		Str("check_name", r.prober.Name()).
		Msg("ad-hoc check done")

	publisher.Publish(adhocData{
		tenantId: tenantId,
		streams: Streams{
			{
				Labels: fmt.Sprintf(`{probe="%s",source="synthetic-monitoring",type="adhoc"}`, r.probe),
				Entries: []logproto.Entry{
					{
						Timestamp: start,
						Line:      buf.String(),
					},
				},
			},
		},
	})

	r.logger.Debug().
		Str("id", r.id).
		Str("target", r.target).
		Str("probe", r.probe).
		Str("check_name", r.prober.Name()).
		Msg("ad-hoc result sent to publisher")
}

type TimeSeries = []prompb.TimeSeries
type Streams = []logproto.Stream

type adhocData struct {
	tenantId model.GlobalID
	streams  Streams
}

func (d adhocData) Metrics() TimeSeries {
	return nil
}

func (d adhocData) Streams() Streams {
	return d.streams
}

func (d adhocData) Tenant() model.GlobalID {
	return d.tenantId
}
