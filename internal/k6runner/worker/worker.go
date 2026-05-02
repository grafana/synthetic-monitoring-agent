// Package worker is the k6-runner-service worker side: it long-polls
// the dispatcher for jobs, runs each job through an [Executor]
// (concurrency = 1 by construction of the loop), and reports the result
// back to the dispatcher.
//
// In phase 1 the [Executor] is backed by [k6runner.Local]. In phase 2 a
// bubblewrap-based sandbox executor implements the same interface
// without any worker-loop changes.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
)

// Executor runs a k6 script and returns the result. The phase 1
// implementation wraps [k6runner.Local]; phase 2 swaps in a
// sandbox-backed implementation behind the same interface.
type Executor interface {
	Run(ctx context.Context, script k6runner.Script, secretStore k6runner.SecretStore) (*k6runner.RunResponse, error)
}

// Config is the worker's runtime configuration.
type Config struct {
	// DispatcherURL is the base URL of the dispatcher (no trailing /run).
	DispatcherURL string
	// Tier is the tier name this worker pulls from. Must match a tier
	// the dispatcher serves.
	Tier string
	// PollTimeout bounds an individual /dequeue request. Should be
	// slightly more than the dispatcher's DequeueHold so the worker
	// always sees the dispatcher's 204 instead of a client-side
	// timeout.
	PollTimeout time.Duration
	// ResultTimeout bounds the /result POST.
	ResultTimeout time.Duration
}

// Defaults applied when [Config] fields are zero.
const (
	DefaultPollTimeout   = 30 * time.Second
	DefaultResultTimeout = 10 * time.Second
)

func (c *Config) withDefaults() Config {
	out := *c
	if out.PollTimeout <= 0 {
		out.PollTimeout = DefaultPollTimeout
	}
	if out.ResultTimeout <= 0 {
		out.ResultTimeout = DefaultResultTimeout
	}
	return out
}

// Worker is the long-poll loop that ties the dispatcher and executor
// together.
type Worker struct {
	cfg      Config
	executor Executor
	client   *http.Client
	logger   zerolog.Logger
	metrics  *Metrics

	// startTime tracks worker process uptime for the cold-start metric.
	startTime time.Time
}

// New constructs a [Worker]. The dispatcher URL must be non-empty and
// the tier must be set; both are programming errors otherwise.
func New(cfg Config, executor Executor, metrics *Metrics, logger zerolog.Logger) (*Worker, error) {
	cfg = cfg.withDefaults()
	if cfg.DispatcherURL == "" {
		return nil, errors.New("worker: DispatcherURL is required")
	}
	if cfg.Tier == "" {
		return nil, errors.New("worker: Tier is required")
	}
	if _, err := url.Parse(cfg.DispatcherURL); err != nil {
		return nil, fmt.Errorf("worker: parsing DispatcherURL: %w", err)
	}
	if executor == nil {
		return nil, errors.New("worker: Executor is required")
	}
	return &Worker{
		cfg:       cfg,
		executor:  executor,
		client:    &http.Client{Timeout: cfg.PollTimeout},
		logger:    logger,
		metrics:   metrics,
		startTime: time.Now(),
	}, nil
}

// Run is the main loop. It returns when ctx is cancelled. Transient
// errors talking to the dispatcher are logged and retried with a small
// backoff; persistent failures should be visible via prometheus
// counters and the readiness gate of whichever process holds the
// worker.
func (w *Worker) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}

		envelope, ok, err := w.poll(ctx)
		switch {
		case err != nil:
			w.logger.Warn().Err(err).Msg("dispatcher poll failed")
			if w.metrics != nil {
				w.metrics.PollErrors.Inc()
			}
			if !sleepCtx(ctx, time.Second) {
				return nil
			}
			continue
		case !ok:
			// Dispatcher had nothing for us; reconnect.
			continue
		}

		w.execute(ctx, envelope)
	}
}

func (w *Worker) execute(ctx context.Context, envelope dequeueEnvelope) {
	if w.metrics != nil {
		w.metrics.JobsExecuted.WithLabelValues(envelope.Tier).Inc()
	}

	logger := w.logger.With().Str("jobID", envelope.JobID).Str("tier", envelope.Tier).Logger()

	// The dispatcher already validated NotAfter at dequeue time; bound the executor by the request's NotAfter so a
	// crashed dispatcher can't keep the worker busy forever.
	execCtx := ctx
	var cancel context.CancelFunc
	if !envelope.Request.NotAfter.IsZero() {
		execCtx, cancel = context.WithDeadline(ctx, envelope.Request.NotAfter)
		defer cancel()
	}

	start := time.Now()
	resp, err := w.executor.Run(execCtx, envelope.Request.Script, envelope.Request.SecretStore)
	duration := time.Since(start)
	if w.metrics != nil {
		w.metrics.RunLatency.WithLabelValues(envelope.Tier).Observe(duration.Seconds())
	}

	if err != nil {
		logger.Warn().Err(err).Dur("duration", duration).Msg("executor returned error")
		// Map executor errors to the worker-crash family. Phase 2 will distinguish
		// pre-script vs mid-script via cgroup events.
		resp = &k6runner.RunResponse{
			ErrorCode: k6runner.ErrorCodeWorkerCrashMidScript,
			Error:     err.Error(),
		}
	}

	if err := w.report(ctx, envelope.JobID, resp); err != nil {
		logger.Error().Err(err).Msg("reporting result to dispatcher")
		if w.metrics != nil {
			w.metrics.ReportErrors.Inc()
		}
	}
}

// dequeueEnvelope mirrors the dispatcher's response shape. We define a
// local copy rather than importing the dispatcher package to keep the
// worker free of dispatcher-internal dependencies.
type dequeueEnvelope struct {
	JobID   string                  `json:"jobID"`
	Tier    string                  `json:"tier"`
	Family  string                  `json:"family"`
	Request k6runner.HTTPRunRequest `json:"request"`
}

func (w *Worker) poll(ctx context.Context) (dequeueEnvelope, bool, error) {
	dequeueURL, err := url.JoinPath(w.cfg.DispatcherURL, "dequeue")
	if err != nil {
		return dequeueEnvelope{}, false, fmt.Errorf("building dequeue URL: %w", err)
	}
	dequeueURL += "?tier=" + url.QueryEscape(w.cfg.Tier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dequeueURL, nil)
	if err != nil {
		return dequeueEnvelope{}, false, fmt.Errorf("building dequeue request: %w", err)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return dequeueEnvelope{}, false, fmt.Errorf("dequeue request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return dequeueEnvelope{}, false, nil
	case http.StatusOK:
		var env dequeueEnvelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			return dequeueEnvelope{}, false, fmt.Errorf("decoding dequeue body: %w", err)
		}
		return env, true, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return dequeueEnvelope{}, false, fmt.Errorf("dequeue returned %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
}

func (w *Worker) report(ctx context.Context, jobID string, response *k6runner.RunResponse) error {
	resultURL, err := url.JoinPath(w.cfg.DispatcherURL, "result", jobID)
	if err != nil {
		return fmt.Errorf("building result URL: %w", err)
	}

	body, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encoding result body: %w", err)
	}

	reportCtx, cancel := context.WithTimeout(ctx, w.cfg.ResultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reportCtx, http.MethodPost, resultURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building result request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("result request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusGone:
		// /run already gave up. Not a worker problem.
		return nil
	default:
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("result returned %d: %s", resp.StatusCode, bytes.TrimSpace(buf))
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// LocalExecutor adapts [k6runner.Runner] (e.g. [k6runner.Local]) to the
// worker's [Executor] interface. The worker's logger is injected via
// [k6runner.Runner.WithLogger] before each call.
type LocalExecutor struct {
	Runner k6runner.Runner
	Logger zerolog.Logger
}

// Run implements [Executor].
func (e LocalExecutor) Run(ctx context.Context, script k6runner.Script, secretStore k6runner.SecretStore) (*k6runner.RunResponse, error) {
	r := e.Runner.WithLogger(&e.Logger)
	return r.Run(ctx, script, secretStore)
}

// Metrics holds the prometheus collectors exposed by the worker.
type Metrics struct {
	// JobsExecuted counts every job pulled and executed.
	JobsExecuted *prometheus.CounterVec
	// RunLatency observes the total time spent in [Executor.Run].
	RunLatency *prometheus.HistogramVec
	// PollErrors counts dispatcher poll failures (transport, decode).
	PollErrors prometheus.Counter
	// ReportErrors counts result-reporting failures.
	ReportErrors prometheus.Counter
}

// NewMetrics registers the worker collectors on r.
func NewMetrics(r prometheus.Registerer) *Metrics {
	m := &Metrics{
		JobsExecuted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "k6_runner",
			Subsystem: "worker",
			Name:      "jobs_executed_total",
			Help:      "Number of jobs the worker has dequeued and run, labelled by tier.",
		}, []string{"tier"}),
		RunLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "k6_runner",
			Subsystem: "worker",
			Name:      "run_duration_seconds",
			Help:      "Duration of the executor.Run call, labelled by tier.",
			Buckets:   prometheus.ExponentialBuckets(0.05, 2, 10),
		}, []string{"tier"}),
		PollErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "k6_runner",
			Subsystem: "worker",
			Name:      "poll_errors_total",
			Help:      "Number of failed /dequeue requests against the dispatcher.",
		}),
		ReportErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "k6_runner",
			Subsystem: "worker",
			Name:      "report_errors_total",
			Help:      "Number of failed /result POSTs against the dispatcher.",
		}),
	}
	r.MustRegister(m.JobsExecuted, m.RunLatency, m.PollErrors, m.ReportErrors)
	return m
}
