package adhoc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	prompb "github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"

	logproto "github.com/grafana/loki/pkg/push"
	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
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
	supportsProtocolSecrets      bool
	isClusterEnabled             bool

	// resultsClient is the client used to report results back to the
	// API. It's set by loop for every connection, and it's only
	// accessed from the goroutine running loop.
	resultsClient sm.AdHocChecksClient
}

// Error represents errors returned from this package.
type Error string

func (e Error) Error() string { return string(e) }

const (
	errNotAuthorized       = Error("probe not authorized")
	errTransportClosing    = Error("transport closing")
	errProbeUnregistered   = Error("probe no longer registered")
	errIncompatibleApi     = Error("API does not support required features")
	errInvalidAdHocRequest = Error("invalid ad-hoc request")
	errIdleTimeout         = Error("connection idle timeout")

	k6AdhocGraceTime = 20 * time.Second

	// reportTimeout is how long to wait for the API to acknowledge the
	// result of an ad-hoc execution.
	reportTimeout = 10 * time.Second
)

// Removed HTTP status code parsing; we treat all codes.Unavailable as idle timeout.

type runner struct {
	logger  zerolog.Logger
	prober  prober.Prober
	id      string
	target  string
	probe   string
	timeout time.Duration

	// results is used to report the outcome of the execution back to
	// the API. If nil, results are only published to Loki.
	results sm.AdHocChecksClient
	probeID int64

	// omitReason keeps the reason a check failed, which is taken from
	// what the prober logged, out of the result. It is set for checks
	// that run k6 scripts, where what is logged is whatever the script
	// printed, which the tenant cannot read from the check configuration.
	omitReason bool
}

// couldNotRunMessage is the error reported when the probe could not run
// a check. The cause is logged by the agent, and not sent, because it
// can contain details of the infrastructure of the probe.
const couldNotRunMessage = "the probe could not run the check"

// usesK6 returns true for the checks that run a k6 script.
func usesK6(t sm.CheckType) bool {
	switch t {
	case sm.CheckTypeMultiHttp, sm.CheckTypeScripted, sm.CheckTypeBrowser:
		return true

	default:
		return false
	}
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
	opsCounter  *prometheus.CounterVec
	k6GraceTime prometheus.Gauge
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
	Conn                    ClientConn
	Logger                  zerolog.Logger
	Backoff                 Backoffer
	Publisher               pusher.Publisher
	TenantCh                chan<- sm.Tenant
	PromRegisterer          prometheus.Registerer
	Features                feature.Collection
	K6Runner                k6runner.Runner
	SecretProvider          secrets.SecretProvider
	SupportsProtocolSecrets bool
	IsClusterEnabled        bool

	// these two fields exists so that tests can pass alternate
	// implementations, they are unexported so that clients of this
	// package always use the default ones.
	runnerFactory                func(context.Context, *sm.AdHocRequest) (*runner, error)
	grpcAdhocChecksClientFactory func(conn ClientConn) (sm.AdHocChecksClient, error)
}

// NewHandler creates a new Handler using the specified options.
func NewHandler(opts HandlerOpts) (*Handler, error) {
	opsCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm",
			Subsystem: "adhoc",
			Name:      "ops_total",
			Help:      "Total number of adhoc operations",
		},
		[]string{"type"},
	)

	k6GraceTime := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "sm",
			Subsystem: "adhoc",
			Name:      "k6_grace_time_seconds",
			Help:      "Grace time added to ad-hoc k6 check timeouts",
		},
	)
	k6GraceTime.Set(k6AdhocGraceTime.Seconds())

	if err := opts.PromRegisterer.Register(opsCounter); err != nil {
		return nil, err
	}

	if err := opts.PromRegisterer.Register(k6GraceTime); err != nil {
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
		proberFactory:                prober.NewProberFactory(opts.K6Runner, 0, opts.Features, opts.SecretProvider),
		supportsProtocolSecrets:      opts.SupportsProtocolSecrets,
		isClusterEnabled:             opts.IsClusterEnabled,
		api: apiInfo{
			conn: opts.Conn,
		},
		metrics: metrics{
			opsCounter:  opsCounter,
			k6GraceTime: k6GraceTime,
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

		case errors.Is(err, errIdleTimeout):
			// connection idle timeout - this is normal behavior when adhoc checks aren't used frequently
			h.logger.Info().
				Str("connection_state", h.api.conn.GetState().String()).
				Msg("connection idle timeout, reconnecting")

			// After idle timeout, reset the backoff counter to start afresh.
			h.backoff.Reset()

			// Sleep to allow context cancellation to be checked
			if err := sleepCtx(ctx, h.backoff.Duration()); err != nil {
				return err
			}

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

	h.resultsClient = client

	grpcErrorHandler := func(action string, err error) error {
		status, ok := status.FromError(err)

		switch {
		case !ok:
			h.logger.Error().Err(err).Bool("ok", ok).Str("action", action).Msg("unknown error")
			return fmt.Errorf("%s: %w", action, err)

		case status.Code() == codes.Canceled:
			// either we were told to shut down
			return context.Canceled

		case status.Message() == "transport is closing":
			// the other end is shutting down
			return errTransportClosing

		case status.Code() == codes.Unavailable:
			// Treat all Unavailable as idle timeout; allow reconnect loop.
			return errIdleTimeout

		case status.Code() == codes.PermissionDenied:
			h.logger.Error().Err(err).Bool("ok", ok).Str("action", action).Uint32("code", uint32(status.Code())).Msg(status.Message())
			return errNotAuthorized

		case status.Code() == codes.Unimplemented:
			h.logger.Error().Err(err).Bool("ok", ok).Str("action", action).Uint32("code", uint32(status.Code())).Msg(status.Message())
			return errIncompatibleApi

		default:
			h.logger.Error().Err(err).Bool("ok", ok).Str("action", action).Uint32("code", uint32(status.Code())).Msg(status.Message())
			return status.Err()
		}
	}

	result, err := client.RegisterProbe(
		ctx,
		&sm.ProbeInfo{
			Version:                 version.Short(),
			Commit:                  version.Commit(),
			Buildstamp:              version.Buildstamp(),
			SupportsProtocolSecrets: h.supportsProtocolSecrets,
			IsClusterEnabled:        h.isClusterEnabled,
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
		// Tell the API that the check never ran, so that the caller can
		// tell this apart from a check that ran and failed.
		//
		// TODO(mem): this should also be published to Loki, so that the
		// frontend is able to communicate to the user that something
		// went wrong.
		reportResult(ctx, h.resultsClient, &sm.AdHocResult{
			ExecutionId: ahReq.AdHocCheck.Id,
			ProbeId:     h.probeID(),
			Status:      sm.AdHocResult_COULD_NOT_START,
			Error:       err.Error(),
		}, h.logger)

		return err
	}

	runner.results = h.resultsClient
	runner.probeID = h.probeID()

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
	// This should never happen. If we hit this, it's a bug in the code
	// that is handling the request, or the API sent us an invalid request.
	if req == nil || req.AdHocCheck.TenantId == 0 {
		return nil, errInvalidAdHocRequest
	}

	check := model.Check{
		Check: sm.Check{
			TenantId: req.AdHocCheck.TenantId,
			Target:   req.AdHocCheck.Target,
			Timeout:  req.AdHocCheck.Timeout,
			Settings: req.AdHocCheck.Settings,
			Channels: req.AdHocCheck.Channels,

			// All the following fields are not used for ad-hoc checks.
			Id:               0, // ad-hoc checks don't have an ID in this sense.
			Job:              "",
			Frequency:        0,
			Offset:           0,
			Enabled:          true,
			Labels:           nil,
			Probes:           nil,
			BasicMetricsOnly: false,
			AlertSensitivity: "",
			Created:          0,
			Modified:         0,
		},
	}

	p, target, err := h.proberFactory.New(ctx, h.logger, check)
	if err != nil {
		return nil, err
	}

	// Ad-hoc checks that rely on k6 runners need some grace time to give the runner time to communicate a timeout back
	// to us, instead of both us and the runner timing out at the same time and missing the runner's response. We define
	// that grace time to be 20 seconds, to be consistent with the grace time used for k6, non ad-hoc checks.
	timeout := time.Duration(check.Timeout) * time.Millisecond
	if usesK6(check.Type()) {
		timeout += k6AdhocGraceTime
	}

	return &runner{
		logger:  h.logger,
		prober:  p,
		id:      req.AdHocCheck.Id,
		target:  target,
		probe:   h.probe.Name,
		timeout: timeout,

		omitReason: usesK6(check.Type()),
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
func (l *jsonLogger) Log(keyvals ...any) error {
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

// probeID returns the ID of the probe as registered with the API, or 0
// if the probe is not registered yet.
func (h *Handler) probeID() int64 {
	if h.probe == nil {
		return 0
	}

	return h.probe.Id
}

// maxReasonBytes is the maximum size of the reason for a failure sent to
// the API.
const maxReasonBytes = 256

// maxResultLogsBytes is the maximum total size of the log lines sent to
// the API with the result of an execution.
const maxResultLogsBytes = 512 * 1024

// encodeLogs encodes the log entries of an execution as JSON lines, in
// the same way they are in the Loki entry. If they do not fit in limit
// bytes, the oldest ones are dropped and truncated is true.
//
// The newest entries are kept, because the cause of a failure is
// usually at the end. A single entry larger than the limit is replaced
// by a short marker, so that it does not push out the newer entries.
func encodeLogs(entries []map[string]string, limit int) (lines [][]byte, truncated bool) {
	var size int

	// Walk from the newest entry to the oldest, and put the result back
	// in order at the end.
	for i := len(entries) - 1; i >= 0; i-- {
		line, err := json.Marshal(entries[i])
		if err != nil {
			continue
		}

		if len(line) > limit {
			line = []byte(fmt.Sprintf(`{"truncated":"log line of %d bytes dropped"}`, len(line)))
			truncated = true
		}

		if size+len(line) > limit {
			truncated = true

			break
		}

		size += len(line)
		lines = append(lines, line)
	}

	slices.Reverse(lines)

	return lines, truncated
}

// failureReason returns a short, human readable reason why a check
// failed, built from what the prober logged, or an empty string if the
// prober did not log any error.
//
// Probers log the cause of a failure as an error entry, right before
// they return, so the last error entry is the one that says why. The
// level is compared ignoring case, because probers don't agree on it.
func failureReason(entries []map[string]string) string {
	for i := len(entries) - 1; i >= 0; i-- {
		if strings.EqualFold(entries[i]["level"], "error") {
			return truncateUTF8(formatReason(entries[i]), maxReasonBytes)
		}
	}

	return ""
}

// formatReason formats a log entry as "message: error (key=value, ...)".
// The level and the time are left out, and the remaining fields are
// sorted so that the result is stable.
func formatReason(entry map[string]string) string {
	var b strings.Builder

	b.WriteString(entry["msg"])

	if err := entry["err"]; err != "" {
		if b.Len() > 0 {
			b.WriteString(": ")
		}

		b.WriteString(err)
	}

	var keys []string

	for k := range entry {
		switch k {
		case "level", "msg", "err", "time":
		default:
			keys = append(keys, k)
		}
	}

	if len(keys) == 0 {
		return b.String()
	}

	sort.Strings(keys)

	if b.Len() > 0 {
		b.WriteString(" ")
	}

	b.WriteString("(")

	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}

		b.WriteString(k + "=" + entry[k])
	}

	b.WriteString(")")

	return b.String()
}

// truncateUTF8 cuts s to at most max bytes, without leaving half of a
// multibyte character at the end.
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}

	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}

	return s[:max]
}

// reportResult tells the API the outcome of an ad-hoc execution. It's
// best effort: if there is no client, or the call fails, the result is
// still available in Loki, so errors are only logged.
func reportResult(ctx context.Context, client sm.AdHocChecksClient, result *sm.AdHocResult, logger zerolog.Logger) {
	if client == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, reportTimeout)
	defer cancel()

	// A long check can outlive the connection it was started on. Wait
	// for the connection to come back, up to reportTimeout, instead of
	// failing right away: by now the result is all that's missing.
	if _, err := client.ReportAdHocResult(ctx, result, grpc.WaitForReady(true)); err != nil {
		// These are expected, and retrying would not help:
		//   - Unimplemented: the API is older, or the feature is off.
		//   - NotFound: the API does not track this execution, which is
		//     the case for every plain "test" run.
		//   - AlreadyExists: the result was already reported.
		//   - FailedPrecondition: the result arrived after the API's
		//     deadline for the execution.
		level := zerolog.WarnLevel

		switch status.Code(err) {
		case codes.Unimplemented, codes.NotFound, codes.AlreadyExists, codes.FailedPrecondition:
			level = zerolog.DebugLevel
		}

		logger.WithLevel(level).Err(err).Str("id", result.ExecutionId).Msg("cannot report ad-hoc result to API, using Loki only")
	}
}

// Run runs the specified prober once and captures the results using
// jsonLogger.
func (r *runner) Run(ctx context.Context, tenantId model.GlobalID, publisher pusher.Publisher) {
	r.logger.Info().Msg("running ad-hoc check")

	var (
		logger   = &jsonLogger{}
		registry = prometheus.NewRegistry()
		// This is the execution ID for the check run.
		executionID = uuid.New().String()
	)

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

	var (
		success  bool
		duration float64
		probeErr error
	)

	// Probers that can tell a check that failed from a check that could
	// not be run say so. For the rest, both are a failure.
	if ep, ok := r.prober.(prober.ErrorProber); ok {
		success, duration, probeErr = ep.ProbeErr(rCtx, r.target, registry, logger, executionID)
	} else {
		success, duration = r.prober.Probe(rCtx, r.target, registry, logger, executionID)
	}

	timedOut := errors.Is(rCtx.Err(), context.DeadlineExceeded)

	// Running out of time is what happened, whatever else the prober
	// reports on its way out. Otherwise the check could not be run.
	couldNotRun := probeErr != nil && !success && !timedOut

	if couldNotRun {
		// The cause can name the infrastructure of the probe: keep it in
		// the log of the agent, and say what happened in the logs of the
		// execution, which are sent.
		r.logger.Error().
			Err(probeErr).
			Str("id", r.id).
			Str("target", r.target).
			Str("probe", r.probe).
			Str("check_name", r.prober.Name()).
			Msg("the probe could not run the check")

		_ = logger.Log("level", "error", "source", "agent", "msg", couldNotRunMessage)
	}

	if success {
		successGauge.Set(1)
	} else {
		successGauge.Set(0)
	}

	if duration != 0 {
		durationGauge.Set(duration)
	} else {
		durationGauge.Set(float64(time.Since(start).Microseconds()) / 1e6)
	}

	mfs, err := registry.Gather()

	buf := &bytes.Buffer{}
	targetLogger := zerolog.New(buf)

	targetLogger.Warn().
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
				Labels: fmt.Sprintf(`{probe=%q,source="synthetic-monitoring",type="adhoc"}`, r.probe),
				Entries: []logproto.Entry{
					{
						Timestamp: start,
						Line:      buf.String(),
						StructuredMetadata: logproto.LabelsAdapter{
							{Name: "execution_id", Value: executionID},
						},
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

	// Report to the API once the check is complete, after publishing, so
	// that the Loki path is never delayed by it.
	result := &sm.AdHocResult{
		ExecutionId:     r.id,
		ProbeId:         r.probeID,
		Status:          sm.AdHocResult_FAILED,
		DurationSeconds: duration,
	}

	if duration == 0 {
		result.DurationSeconds = time.Since(start).Seconds()
	}

	switch {
	case success:
		result.Status = sm.AdHocResult_OK
	case timedOut:
		result.Status = sm.AdHocResult_TIMEOUT
	case couldNotRun:
		result.Status = sm.AdHocResult_COULD_NOT_START
	}

	switch {
	case err != nil:
		result.Error = err.Error()

	case couldNotRun:
		result.Error = couldNotRunMessage

	case result.Status != sm.AdHocResult_OK && !r.omitReason:
		// The caller should be able to tell why the check did not pass.
		result.Error = failureReason(logger.entries)
	}

	// A check that passed has nothing to explain. Otherwise, send the
	// same lines that go to Loki, so that the caller can see what
	// happened without querying it.
	if result.Status != sm.AdHocResult_OK {
		result.Logs, result.LogsTruncated = encodeLogs(logger.entries, maxResultLogsBytes)
	}

	reportResult(ctx, r.results, result, r.logger)
}

type (
	TimeSeries = []prompb.TimeSeries
	Streams    = []logproto.Stream
)

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
