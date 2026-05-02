// Package dispatcher implements the front-end of the k6 runner service:
// it accepts /run requests from the agent, infers the target tier, holds
// the request in a per-tier in-memory queue waiting for a worker, and
// returns the worker's result when it arrives.
//
// Workers pull work via /dequeue?tier=X (long-poll) and report results
// via /result/{id}. This pull model — instead of dispatcher-driven push
// — means the dispatcher does not need to track worker presence: the
// presence of an outstanding /dequeue is the readiness signal.
//
// Drain semantics: when [Dispatcher.Drain] is called, /run stops
// accepting new requests and any jobs still waiting in a queue are
// returned to their callers tagged with [k6runner.ErrorCodeDispatcherDrain].
// The agent recognises this and reschedules immediately without
// counting the outcome as a check failure.
package dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner/tiermap"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner/version"
)

// Repository is the subset of [version.Repository] the dispatcher needs to back the /versions and
// /versions/resolve endpoints. An interface keeps tests free of filesystem fixtures.
type Repository interface {
	Entries() ([]version.Entry, error)
	Resolve(constraint string) (*version.Entry, error)
}

// Defaults applied when [Config] fields are zero.
const (
	DefaultHold          = 2 * time.Second
	DefaultQueueCapacity = 1024
	DefaultDequeueHold   = 25 * time.Second
)

// Config holds dispatcher configuration. Zero-valued fields fall back
// to package defaults.
type Config struct {
	// Hold bounds how long /run will wait for a worker to pick up a queued
	// request before returning ErrorCodeDispatchCapacity. Per the design
	// spec, default is 2s.
	Hold time.Duration
	// DequeueHold bounds how long /dequeue waits for an item before
	// returning 204. Slightly less than the worker's HTTP read timeout.
	DequeueHold time.Duration
	// QueueCapacity is the per-tier queue capacity. Overflow returns
	// ErrorCodeDispatchCapacity to /run immediately.
	QueueCapacity int
	// Tiers is the list of deployed tier names. /run requests routed to
	// a tier not in this list increment tenant_tier_mapping_errors_total
	// and return an error to the agent.
	Tiers []string
	// Repository backs the /versions and /versions/resolve endpoints. May be nil; in that case the endpoints return 503.
	Repository Repository
}

func (c *Config) withDefaults() Config {
	out := *c
	if out.Hold <= 0 {
		out.Hold = DefaultHold
	}
	if out.DequeueHold <= 0 {
		out.DequeueHold = DefaultDequeueHold
	}
	if out.QueueCapacity <= 0 {
		out.QueueCapacity = DefaultQueueCapacity
	}
	return out
}

// Dispatcher routes incoming /run requests to per-tier queues and
// matches results coming back from workers to waiting clients.
type Dispatcher struct {
	cfg        Config
	mapping    *tiermap.Live
	logger     zerolog.Logger
	metrics    *Metrics
	repository Repository

	queues   map[string]*tierQueue // tier name -> queue
	inflight sync.Map              // job ID -> *job

	draining atomic.Bool
}

// New constructs a [Dispatcher]. The supplied [tiermap.Live] is used to
// resolve incoming requests to a tier; metrics may be nil to use a
// noop registry.
func New(cfg Config, mapping *tiermap.Live, metrics *Metrics, logger zerolog.Logger) (*Dispatcher, error) {
	if mapping == nil {
		return nil, errors.New("dispatcher: mapping is required")
	}
	cfg = cfg.withDefaults()
	if len(cfg.Tiers) == 0 {
		return nil, errors.New("dispatcher: at least one tier must be configured")
	}

	queues := make(map[string]*tierQueue, len(cfg.Tiers))
	for _, t := range cfg.Tiers {
		queues[t] = newTierQueue(t, cfg.QueueCapacity)
	}

	return &Dispatcher{
		cfg:        cfg,
		mapping:    mapping,
		logger:     logger,
		metrics:    metrics,
		repository: cfg.Repository,
		queues:     queues,
	}, nil
}

// Drain stops accepting new /run requests and returns any queued jobs
// to their callers tagged as [k6runner.ErrorCodeDispatcherDrain]. Drain
// blocks until either every queue is empty or ctx is cancelled.
// Inflight jobs (already pulled by a worker) are not interrupted; the
// caller is responsible for waiting on them through normal request
// handling before shutting the HTTP server.
func (d *Dispatcher) Drain(ctx context.Context) {
	if !d.draining.CompareAndSwap(false, true) {
		return
	}
	d.logger.Info().Msg("dispatcher draining; new /run requests will be rejected")

	for _, q := range d.queues {
		q.drainPending()
	}

	// Best-effort wait for inflight jobs to settle. A crashed worker
	// will never deliver a result; we let ctx cap the wait.
	deadline := time.NewTimer(time.Until(deadlineFromContext(ctx, 30*time.Second)))
	defer deadline.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		case <-time.After(50 * time.Millisecond):
			if d.inflightCount() == 0 {
				return
			}
		}
	}
}

func (d *Dispatcher) inflightCount() int {
	n := 0
	d.inflight.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

func deadlineFromContext(ctx context.Context, fallback time.Duration) time.Time {
	if dl, ok := ctx.Deadline(); ok {
		return dl
	}
	return time.Now().Add(fallback)
}

// Handler returns an http.Handler that mounts the dispatcher's routes:
//
//   - POST /run                        agent  → dispatcher: submit a check
//   - POST /dequeue?tier=X             worker → dispatcher: pull a job
//   - POST /result/{id}                worker → dispatcher: deliver a result
//   - GET  /versions                   agent  → dispatcher: list installed k6 binaries
//   - GET  /versions/resolve?manifest= agent  → dispatcher: resolve a semver constraint
func (d *Dispatcher) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /run", d.handleRun)
	mux.HandleFunc("POST /dequeue", d.handleDequeue)
	mux.HandleFunc("POST /result/{id}", d.handleResult)
	mux.HandleFunc("GET /versions", d.handleVersions)
	mux.HandleFunc("GET /versions/resolve", d.handleVersionsResolve)
	return mux
}

// handleRun is the agent-facing endpoint. The request body is an
// [k6runner.HTTPRunRequest]; the response body is an
// [k6runner.RunResponse]. Failure modes use the wire-protocol error
// codes defined in the parent package.
func (d *Dispatcher) handleRun(w http.ResponseWriter, r *http.Request) {
	if d.draining.Load() {
		writeDrain(w)
		return
	}

	requestID := r.Header.Get("X-Request-Id")
	logger := d.logger
	if requestID != "" {
		logger = logger.With().Str("clientRequestID", requestID).Logger()
	}

	var req k6runner.HTTPRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decoding request: %v", err), http.StatusBadRequest)
		return
	}

	// Match legacy sm-k6-runner /run rewrites (docs/k6-runner-api.yaml):
	//   - empty k6ChannelManifest is treated as "*" (any installed binary).
	//   - notAfter values that are clearly bogus (>1h in the future, or >7d in the past — typically
	//     a missing/zero value from an old client) are replaced with a sane fallback.
	if req.K6ChannelManifest == "" {
		req.K6ChannelManifest = "*"
	}
	d.clampNotAfter(&req)

	tenantID := tenantIDFrom(req)
	tier, family, err := d.mapping.Tier(req.CheckInfo.Type, tenantID)
	if err != nil {
		http.Error(w, fmt.Sprintf("resolving tier: %v", err), http.StatusBadRequest)
		return
	}

	queue, known := d.queues[tier]
	if !known {
		// Mapping points at a tier that has no deployed worker pool. The legacy proxy returns 422 with no body
		// for "script type is not routable in this proxy"; mirror that.
		if d.metrics != nil {
			d.metrics.MappingErrors.WithLabelValues(tiermap.CauseUnknownTier).Inc()
		}
		logger.Warn().Str("tier", tier).Str("type", req.CheckInfo.Type).
			Msg("tier has no deployed worker pool; rejecting with 422")
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	job := &job{
		id:         uuid.NewString(),
		tier:       tier,
		family:     family,
		request:    req,
		enqueued:   time.Now(),
		dispatched: make(chan struct{}),
		result:     make(chan jobResult, 1),
	}

	if !queue.tryEnqueue(job) {
		// Queue full → no worker capacity at all. 503 dispatch_capacity.
		if d.metrics != nil {
			d.metrics.FailureCauses.WithLabelValues(tier, k6runner.ErrorCodeDispatchCapacity).Inc()
		}
		writeRunResponseError(w, http.StatusServiceUnavailable, k6runner.ErrorCodeDispatchCapacity,
			"dispatcher queue full")
		return
	}

	d.inflight.Store(job.id, job)
	defer d.inflight.Delete(job.id)
	if d.metrics != nil {
		d.metrics.QueueDepth.WithLabelValues(tier).Set(float64(queue.depth()))
	}

	// Wait for one of: result delivered, dispatched + result deadline, hold expired without dispatch, client cancel.
	holdTimer := time.NewTimer(d.cfg.Hold)
	defer holdTimer.Stop()

	select {
	case res := <-job.result:
		if d.metrics != nil {
			d.metrics.QueueDepth.WithLabelValues(tier).Set(float64(queue.depth()))
		}
		writeResult(w, res)
		return

	case <-job.dispatched:
		// Worker accepted the job; hold no longer applies. Wait for result without further server-side timeout
		// (the request context still bounds total time).
		holdTimer.Stop()
		select {
		case res := <-job.result:
			if d.metrics != nil {
				d.metrics.QueueDepth.WithLabelValues(tier).Set(float64(queue.depth()))
			}
			writeResult(w, res)
			return
		case <-r.Context().Done():
			// Client gave up while a worker was running the script.
			writeRunResponseError(w, http.StatusGatewayTimeout, k6runner.ErrorCodeWorkerCrashMidScript,
				"client cancelled while worker was executing")
			return
		}

	case <-holdTimer.C:
		// No worker pulled the job within the hold. Mark abandoned so a worker that dequeues later just drops it.
		queue.markAbandoned(job)
		if d.metrics != nil {
			d.metrics.FailureCauses.WithLabelValues(tier, k6runner.ErrorCodeDispatchCapacity).Inc()
			d.metrics.QueueDepth.WithLabelValues(tier).Set(float64(queue.depth()))
		}
		writeRunResponseError(w, http.StatusServiceUnavailable, k6runner.ErrorCodeDispatchCapacity,
			"no worker available within hold")
		return

	case <-r.Context().Done():
		queue.markAbandoned(job)
		http.Error(w, "client cancelled", http.StatusGatewayTimeout)
		return
	}
}

// handleDequeue is the worker-facing long-poll endpoint. Workers pass
// ?tier=<name> and either receive a job (200 with the run request body)
// or a 204 if the tier is empty within the hold.
func (d *Dispatcher) handleDequeue(w http.ResponseWriter, r *http.Request) {
	tier := r.URL.Query().Get("tier")
	queue, known := d.queues[tier]
	if !known {
		http.Error(w, fmt.Sprintf("unknown tier %q", tier), http.StatusBadRequest)
		return
	}

	// Use the smaller of the configured dequeue hold and the request context's remaining time.
	pollCtx, cancel := context.WithTimeout(r.Context(), d.cfg.DequeueHold)
	defer cancel()

	for {
		j, ok := queue.dequeue(pollCtx)
		if !ok {
			// Either timeout or context cancelled; respond 204 so the worker reconnects.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Skip jobs whose NotAfter is past or that were abandoned by /run.
		if j.isAbandoned() {
			continue
		}
		if !j.request.NotAfter.IsZero() && time.Now().After(j.request.NotAfter) {
			j.deliver(jobResult{
				response: &k6runner.RunResponse{
					ErrorCode: k6runner.ErrorCodeDispatchCapacity,
					Error:     "request expired before dispatch (NotAfter passed)",
				},
			})
			continue
		}

		// Mark as dispatched so /run stops counting against the hold.
		j.markDispatched()

		envelope := dequeueEnvelope{
			JobID:   j.id,
			Tier:    j.tier,
			Family:  j.family,
			Request: j.request,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(envelope); err != nil {
			d.logger.Error().Err(err).Str("jobID", j.id).Msg("encoding dequeue response")
		}
		return
	}
}

// handleResult delivers a worker's result to the waiting /run handler.
// The worker POSTs the [k6runner.RunResponse] body; if no waiter is
// registered for the given ID (e.g. /run already returned to the agent
// after a hold timeout) the result is silently discarded.
func (d *Dispatcher) handleResult(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	v, ok := d.inflight.Load(id)
	if !ok {
		// /run already gave up; tell the worker so it can move on.
		http.Error(w, "no waiter for id", http.StatusGone)
		return
	}
	j := v.(*job)

	var resp k6runner.RunResponse
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		http.Error(w, fmt.Sprintf("decoding response: %v", err), http.StatusBadRequest)
		return
	}

	j.deliver(jobResult{response: &resp})
	w.WriteHeader(http.StatusNoContent)
}

// versionsResponse is the wire shape returned by /versions, matching the
// VersionsList schema in docs/k6-runner-api.yaml. The Versions field may be
// nil when the repository is empty.
type versionsResponse struct {
	Versions []version.Entry `json:"versions"`
}

func (d *Dispatcher) handleVersions(w http.ResponseWriter, _ *http.Request) {
	if d.repository == nil {
		http.Error(w, "version repository not configured", http.StatusServiceUnavailable)
		return
	}
	entries, err := d.repository.Entries()
	if err != nil {
		http.Error(w, fmt.Sprintf("scanning k6 repository: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(versionsResponse{Versions: entries}) //nolint:errchkjson // best-effort write
}

func (d *Dispatcher) handleVersionsResolve(w http.ResponseWriter, r *http.Request) {
	if d.repository == nil {
		http.Error(w, "version repository not configured", http.StatusServiceUnavailable)
		return
	}
	manifest := r.URL.Query().Get("manifest")
	if manifest == "" {
		http.Error(w, "missing manifest query parameter", http.StatusBadRequest)
		return
	}
	entry, err := d.repository.Resolve(manifest)
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entry) //nolint:errchkjson // best-effort write
	case errors.Is(err, version.ErrUnsatisfiable):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, version.ErrInvalidConstraint):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		http.Error(w, fmt.Sprintf("resolving manifest: %v", err), http.StatusInternalServerError)
	}
}

// dequeueEnvelope is the wire shape returned to workers from /dequeue.
// The job ID is required so the worker can address the right /result/{id}
// endpoint when it reports back.
type dequeueEnvelope struct {
	JobID   string                  `json:"jobID"`
	Tier    string                  `json:"tier"`
	Family  string                  `json:"family"`
	Request k6runner.HTTPRunRequest `json:"request"`
}

// job is the dispatcher-internal record of a request in flight.
type job struct {
	id         string
	tier       string
	family     string
	request    k6runner.HTTPRunRequest
	enqueued   time.Time
	dispatched chan struct{}
	dispOnce   sync.Once
	result     chan jobResult
	abandoned  atomic.Bool
}

func (j *job) markDispatched() {
	j.dispOnce.Do(func() { close(j.dispatched) })
}

func (j *job) deliver(res jobResult) {
	select {
	case j.result <- res:
	default:
		// /run already moved on (hold timeout, drain, client cancel). The buffered slot of 1 is for the common
		// case where /run is still waiting; if it's full or the channel is no longer being read, drop.
	}
}

func (j *job) isAbandoned() bool { return j.abandoned.Load() }

type jobResult struct {
	response *k6runner.RunResponse
	err      error
}

// tierQueue is the per-tier in-memory queue. The implementation uses a
// buffered channel for O(1) enqueue/dequeue with an explicit count so
// /run can fast-fail on overflow without blocking.
type tierQueue struct {
	name     string
	capacity int

	mu      sync.Mutex
	pending []*job
	cond    *sync.Cond
	closed  bool
}

func newTierQueue(name string, capacity int) *tierQueue {
	q := &tierQueue{name: name, capacity: capacity}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *tierQueue) depth() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

func (q *tierQueue) tryEnqueue(j *job) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	if len(q.pending) >= q.capacity {
		return false
	}
	q.pending = append(q.pending, j)
	q.cond.Signal()
	return true
}

// dequeue blocks until a job is available, ctx is cancelled, or the queue is closed. It skips abandoned jobs.
// Returns the dequeued job and true on success; ok=false on cancellation or close.
func (q *tierQueue) dequeue(ctx context.Context) (*job, bool) {
	// Wake the waiter if ctx is cancelled. We don't have channel-based
	// cond.Wait, so we fan a goroutine that broadcasts on cancel.
	wakeStop := make(chan struct{})
	defer close(wakeStop)
	go func() {
		select {
		case <-ctx.Done():
			q.mu.Lock()
			q.cond.Broadcast()
			q.mu.Unlock()
		case <-wakeStop:
		}
	}()

	q.mu.Lock()
	defer q.mu.Unlock()

	for {
		if ctx.Err() != nil {
			return nil, false
		}
		if q.closed {
			return nil, false
		}
		// Skip leading abandoned entries.
		for len(q.pending) > 0 && q.pending[0].isAbandoned() {
			q.pending = q.pending[1:]
		}
		if len(q.pending) > 0 {
			j := q.pending[0]
			q.pending = q.pending[1:]
			return j, true
		}
		q.cond.Wait()
	}
}

// markAbandoned tags a job as no longer wanted; if it's still in the queue
// the next dequeue will skip it.
func (q *tierQueue) markAbandoned(j *job) {
	j.abandoned.Store(true)
}

// drainPending pulls every queued job out and delivers a drain result
// to each. Subsequent enqueues fail.
func (q *tierQueue) drainPending() {
	q.mu.Lock()
	q.closed = true
	pending := q.pending
	q.pending = nil
	q.cond.Broadcast()
	q.mu.Unlock()

	for _, j := range pending {
		if j.isAbandoned() {
			continue
		}
		j.deliver(jobResult{response: &k6runner.RunResponse{
			ErrorCode: k6runner.ErrorCodeDispatcherDrain,
			Error:     "dispatcher draining; reschedule immediately",
		}})
	}
}

// clampNotAfter mirrors the legacy sm-k6-runner proxy: NotAfter values that are clearly bogus —
// more than one hour in the future, or more than 7 days in the past (typically a zero value from
// an old client) — are replaced with `now + timeout + 2 minutes`. See docs/k6-runner-api.yaml
// /run "notAfter clamping".
func (d *Dispatcher) clampNotAfter(req *k6runner.HTTPRunRequest) {
	const (
		futureBound = 1 * time.Hour
		pastBound   = 7 * 24 * time.Hour
		fallbackPad = 2 * time.Minute
	)
	now := time.Now()
	timeout := time.Duration(req.Settings.Timeout) * time.Millisecond
	switch {
	case req.NotAfter.Sub(now) > futureBound:
		fallthrough
	case now.Sub(req.NotAfter) > pastBound:
		req.NotAfter = now.Add(timeout + fallbackPad)
	}
}

func tenantIDFrom(req k6runner.HTTPRunRequest) string {
	v, ok := req.CheckInfo.Metadata["tenantID"]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64: // JSON numbers decode as float64 by default.
		return fmt.Sprintf("%d", int64(t))
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func writeResult(w http.ResponseWriter, res jobResult) {
	w.Header().Set("Content-Type", "application/json")
	if res.response == nil && res.err != nil {
		writeRunResponseError(w, http.StatusInternalServerError, k6runner.ErrorCodeUnknown, res.err.Error())
		return
	}

	// Status-code mapping: legacy codes follow docs/k6-runner-api.yaml's RunErrorCode table verbatim.
	// New dispatcher-specific codes (dispatch_capacity, dispatcher_drain, worker_crash_*, sandbox_isolation_error)
	// keep their dispatcher-defined mappings — they have no legacy equivalent.
	status := http.StatusOK
	switch res.response.ErrorCode {
	case k6runner.ErrorCodeNone, k6runner.ErrorCodeFailed, k6runner.ErrorCodeAborted:
		status = http.StatusOK
	case k6runner.ErrorCodeTimeout:
		status = http.StatusRequestTimeout
	case k6runner.ErrorCodeKilled,
		k6runner.ErrorCodeUnsupportedVersion,
		k6runner.ErrorCodeBadVersion:
		status = http.StatusUnprocessableEntity
	case k6runner.ErrorCodeBrowser:
		status = http.StatusServiceUnavailable
	case k6runner.ErrorCodeUnknown:
		status = http.StatusInternalServerError
	case k6runner.ErrorCodeDispatchCapacity, k6runner.ErrorCodeDispatcherDrain:
		status = http.StatusServiceUnavailable
		if res.response.ErrorCode == k6runner.ErrorCodeDispatcherDrain {
			w.Header().Set(k6runner.DrainHeader, "1")
			w.Header().Set("Retry-After", "0")
		}
	case k6runner.ErrorCodeWorkerCrashPreScript, k6runner.ErrorCodeWorkerCrashMidScript,
		k6runner.ErrorCodeSandboxIsolation:
		status = http.StatusInternalServerError
	}

	w.WriteHeader(status)
	// Encoding errors at this point mean the client connection went away mid-flight; we already sent the status, so
	// there is nothing meaningful to recover.
	_ = json.NewEncoder(w).Encode(res.response) //nolint:errchkjson // RunResponse is safe; writer errors are logged by http server
}

func writeRunResponseError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	if code == k6runner.ErrorCodeDispatcherDrain {
		w.Header().Set(k6runner.DrainHeader, "1")
		w.Header().Set("Retry-After", "0")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(k6runner.RunResponse{ //nolint:errchkjson // see writeResult
		ErrorCode: code,
		Error:     msg,
	})
}

func writeDrain(w http.ResponseWriter) {
	writeRunResponseError(w, http.StatusServiceUnavailable, k6runner.ErrorCodeDispatcherDrain,
		"dispatcher draining; reschedule immediately")
}
