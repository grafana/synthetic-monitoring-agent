// Package browser provides a client for a pool of crocochrome instances
// serving remote browser sessions for browser checks.
package browser

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jpillora/backoff"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
)

// ErrPoolExhausted is returned by Acquire when no instance could be acquired
// before the context expired.
var ErrPoolExhausted = errors.New("browser pool exhausted")

const (
	metricNamespace = "sm_agent"
	metricSubsystem = "browser_pool"

	defaultSyncInterval = 15 * time.Second

	// acquireAttemptTimeout bounds a single acquire request to an instance,
	// which launches a Chromium process before responding.
	acquireAttemptTimeout = 10 * time.Second
	// releaseTimeout bounds the session delete request on release.
	releaseTimeout = 10 * time.Second
	// syncRequestTimeout bounds each GET /sessions request of a sync tick.
	syncRequestTimeout = 5 * time.Second

	backoffMin = 250 * time.Millisecond
	backoffMax = 2 * time.Second
)

// Config configures a Pool.
type Config struct {
	// URL is the browser pool URL. Its host is DNS-expanded to the set of
	// crocochrome instances by the sync loop.
	URL string
	// SyncInterval is the period of the sync loop. Defaults to 15s.
	SyncInterval time.Duration
	// HTTPClient is the client used to talk to crocochrome instances.
	// Defaults to a client without a global timeout; requests are bounded
	// per-attempt.
	HTTPClient *http.Client
	Logger     zerolog.Logger

	// resolveFleet returns the current set of instance base URLs. It defaults
	// to DNS-expanding URL's host, and exists as a field so tests can inject
	// a static fleet.
	resolveFleet func() ([]string, error)
}

// Pool tracks a fleet of single-session crocochrome instances and allocates
// browser sessions from it. A single Pool is shared by all concurrent check
// executions; all state updates are serialized under its mutex.
type Pool struct {
	cfg     Config
	client  *http.Client
	logger  zerolog.Logger
	metrics metrics

	mtx sync.Mutex
	// order is a self-organizing list of *instance: the instance at the front
	// is the most likely to be free. Instances move to the back when observed
	// busy or allocated, and to the front when released.
	order *list.List
	// elements indexes list elements by instance base URL.
	elements map[string]*list.Element
}

// instance is the pool's view of a single crocochrome instance.
type instance struct {
	baseURL string
	// busy marks an instance claimed by this agent: an Acquire is probing it,
	// or we hold its session. Busy instances are not allocatable and are
	// exempt from sync reordering.
	busy      bool
	sessionID string
}

// metrics holds the pool's Prometheus metrics. The instances gauges are
// GaugeFuncs computing from the pool state under its mutex, so they cannot
// drift from it.
type metrics struct {
	acquires        *prometheus.CounterVec
	probes          *prometheus.CounterVec
	releases        *prometheus.CounterVec
	syncs           *prometheus.CounterVec
	acquireDuration prometheus.Histogram
	instancesBusy   prometheus.GaugeFunc
	instancesFree   prometheus.GaugeFunc
}

// sessionInfo mirrors the relevant parts of crocochrome's session creation
// response.
type sessionInfo struct {
	ID              string `json:"id"`
	ChromiumVersion struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	} `json:"chromiumVersion"`
}

// New creates a Pool from cfg, registers its metrics with registerer, and
// starts its sync loop, which keeps the fleet membership and status up to
// date until ctx is cancelled.
func New(ctx context.Context, cfg Config, registerer prometheus.Registerer) (*Pool, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing browser pool URL %q: %w", cfg.URL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("browser pool URL %q must include scheme and host", cfg.URL)
	}

	if cfg.SyncInterval == 0 {
		cfg.SyncInterval = defaultSyncInterval
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	if cfg.resolveFleet == nil {
		cfg.resolveFleet = fleetResolver(u)
	}
	if registerer == nil {
		registerer = prometheus.NewRegistry() // Empty, unused.
	}

	p := &Pool{
		cfg:      cfg,
		client:   cfg.HTTPClient,
		logger:   cfg.Logger,
		order:    list.New(),
		elements: make(map[string]*list.Element),
	}

	p.metrics = registerMetrics(registerer, p)

	go p.sync(ctx)

	return p, nil
}

// fleetResolver returns the default fleet resolution for a pool URL:
// A literal IP host is the single instance, a hostname is DNS-expanded to every
// A/AAAA record (a headless service resolves to all of its backing pods),
// each combined with the URL's scheme and port.
func fleetResolver(u *url.URL) func() ([]string, error) {
	return func() ([]string, error) {
		host := u.Hostname()
		if net.ParseIP(host) != nil {
			return []string{u.Scheme + "://" + u.Host}, nil
		}

		ips, err := net.LookupHost(host)
		if err != nil {
			return nil, fmt.Errorf("resolving browser pool host %q: %w", host, err)
		}

		return instanceBaseURLs(u.Scheme, u.Port(), ips), nil
	}
}

// instanceBaseURLs combines resolved IPs with the pool URL's scheme and port
// into instance base URLs.
func instanceBaseURLs(scheme, port string, ips []string) []string {
	urls := make([]string, 0, len(ips))

	for _, ip := range ips {
		host := ip
		switch {
		case port != "":
			host = net.JoinHostPort(ip, port)
		case strings.Contains(ip, ":"):
			// IPv6 literals must be bracketed even without a port.
			host = "[" + ip + "]"
		}
		urls = append(urls, scheme+"://"+host)
	}

	return urls
}

// Acquire allocates a browser session from the pool, returning its CDP
// WebSocket URL and a release function that must be called when the session is
// no longer needed. Acquire probes instances in most-likely-free-first order,
// backing off between passes over the fleet, until it succeeds or ctx expires,
// in which case it returns an error wrapping ErrPoolExhausted.
//
// checkInfo is JSON-marshaled as the acquire request body; crocochrome uses it
// for log correlation, the same way the remote k6 runner forwards it in the
// public-probe path.
func (p *Pool) Acquire(ctx context.Context, checkInfo k6runner.CheckInfo) (wsURL string, release func(context.Context), err error) {
	start := time.Now()
	defer func() {
		p.metrics.acquireDuration.Observe(time.Since(start).Seconds())

		switch {
		case err == nil:
			p.metrics.acquires.WithLabelValues("success").Inc()
		case errors.Is(err, ErrPoolExhausted):
			p.metrics.acquires.WithLabelValues("exhausted").Inc()
		default:
			p.metrics.acquires.WithLabelValues("error").Inc()
		}
	}()

	body, err := json.Marshal(checkInfo)
	if err != nil {
		return "", nil, fmt.Errorf("marshaling check info: %w", err)
	}

	bo := backoff.Backoff{Min: backoffMin, Max: backoffMax, Jitter: true}

	for {
		// A pass probes at most one full sweep of the fleet. Failed probes
		// move to the back, so claiming the frontmost non-busy instance visits
		// distinct instances; the budget tells us when we have gone around and
		// should back off instead of hammering a busy fleet.
		for budget := p.size(); budget > 0; budget-- {
			if err := ctx.Err(); err != nil {
				return "", nil, fmt.Errorf("%w: %w", ErrPoolExhausted, err)
			}

			inst := p.claimNext()
			if inst == nil {
				// Every instance is claimed by another Acquire on this agent.
				break
			}

			wsURL, sessionID, err := p.probe(ctx, inst.baseURL, body)
			if err != nil {
				p.logger.Debug().Err(err).Str("instance", inst.baseURL).Msg("browser instance not acquired")

				// Release the claim even when the instance reported busy
				// (409): busy means "claimed by this agent", and only our own
				// release clears it — another agent's session ending would
				// not. The instance being occupied is instead recorded by
				// sending it to the back of the list.
				p.mtx.Lock()
				inst.busy = false
				p.moveToBack(inst)
				p.mtx.Unlock()

				continue
			}

			p.mtx.Lock()
			// busy remains set for as long as we hold the session.
			inst.sessionID = sessionID
			p.moveToBack(inst)
			p.mtx.Unlock()

			p.logger.Debug().Str("instance", inst.baseURL).Str("sessionID", sessionID).Msg("browser session acquired")

			return wsURL, p.releaseFunc(inst, sessionID), nil
		}

		// Pass over the fleet complete without success: back off and start a
		// new one.
		select {
		case <-ctx.Done():
			return "", nil, fmt.Errorf("%w: %w", ErrPoolExhausted, ctx.Err())
		case <-time.After(bo.Duration()):
		}
	}
}

// claimNext claims the frontmost instance that is not busy, marking it busy so
// no other Acquire probes it concurrently. It returns nil when there is no
// claimable instance.
func (p *Pool) claimNext() *instance {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for el := p.order.Front(); el != nil; el = el.Next() {
		inst := el.Value.(*instance)
		if inst.busy {
			continue
		}
		// Claim the instance so concurrent Acquire calls on this agent do not
		// race for it. Cross-agent races are resolved by crocochrome's 409.
		inst.busy = true
		return inst
	}

	return nil
}

// size returns the current number of instances in the pool.
func (p *Pool) size() int {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	return p.order.Len()
}

// probe attempts to acquire a session on a single instance.
func (p *Pool) probe(ctx context.Context, baseURL string, checkInfo []byte) (wsURL, sessionID string, err error) {
	endpoint, err := url.JoinPath(baseURL, "/sessions/acquire")
	if err != nil {
		return "", "", fmt.Errorf("building acquire URL: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, acquireAttemptTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(checkInfo))
	if err != nil {
		return "", "", fmt.Errorf("building acquire request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		p.metrics.probes.WithLabelValues("error").Inc()
		return "", "", fmt.Errorf("requesting session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 409 (busy) and 503 (draining) are expected; anything else is only a
		// log and metric distinction, the instance is skipped all the same.
		switch resp.StatusCode {
		case http.StatusConflict:
			p.metrics.probes.WithLabelValues("busy").Inc()
		case http.StatusServiceUnavailable:
			p.metrics.probes.WithLabelValues("draining").Inc()
		default:
			p.metrics.probes.WithLabelValues("error").Inc()
		}
		return "", "", fmt.Errorf("instance responded %d", resp.StatusCode)
	}

	var si sessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&si); err != nil {
		p.metrics.probes.WithLabelValues("error").Inc()
		return "", "", fmt.Errorf("decoding session: %w", err)
	}
	if si.ID == "" || si.ChromiumVersion.WebSocketDebuggerURL == "" {
		// A session may have been created even though the response is
		// unusable: best-effort delete to avoid leaking it until the
		// instance's session timeout.
		if si.ID != "" {
			dctx, dcancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
			defer dcancel()
			if derr := p.deleteSession(dctx, baseURL, si.ID); derr != nil {
				p.logger.Warn().Err(derr).Str("instance", baseURL).Msg("deleting unusable browser session")
			}
		}
		p.metrics.probes.WithLabelValues("error").Inc()
		return "", "", errors.New("session response missing id or webSocketDebuggerUrl")
	}

	p.metrics.probes.WithLabelValues("acquired").Inc()

	return si.ChromiumVersion.WebSocketDebuggerURL, si.ID, nil
}

// releaseFunc builds the idempotent release function for an acquired session:
// it deletes the session on its instance and returns the instance to the front
// of the list, unconditionally. A failed delete is only logged: a wrong free
// guess is corrected by the next probe's 409, and the instance's session
// timeout reclaims the session.
func (p *Pool) releaseFunc(inst *instance, sessionID string) func(context.Context) {
	var once sync.Once

	return func(ctx context.Context) {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(ctx, releaseTimeout)
			defer cancel()

			if err := p.deleteSession(ctx, inst.baseURL, sessionID); err != nil {
				p.metrics.releases.WithLabelValues("error").Inc()
				p.logger.Warn().Err(err).Str("instance", inst.baseURL).Str("sessionID", sessionID).
					Msg("releasing browser session")
			} else {
				p.metrics.releases.WithLabelValues("ok").Inc()
			}

			p.mtx.Lock()
			inst.sessionID = ""
			inst.busy = false
			p.moveToFront(inst)
			p.mtx.Unlock()
		})
	}
}

func (p *Pool) deleteSession(ctx context.Context, baseURL, sessionID string) error {
	endpoint, err := url.JoinPath(baseURL, "/sessions", sessionID)
	if err != nil {
		return fmt.Errorf("building delete URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("building delete request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("requesting session delete: %w", err)
	}
	defer resp.Body.Close()

	// 404 means the session is already gone (timed out or instance restarted),
	// which is as released as it gets.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("instance responded %d", resp.StatusCode)
	}

	return nil
}

// sync drives the periodic sync loop until ctx is cancelled. It is started by
// New and runs one sync immediately, so membership is populated as soon as
// the pool is constructed rather than after the first interval.
func (p *Pool) sync(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	p.syncOnce(ctx)

	ticker := time.NewTicker(p.cfg.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.syncOnce(ctx)
		}
	}
}

// syncOnce performs one sync tick: it resolves the current fleet, observes
// every instance's busy/free state via GET /sessions, and applies the result
// to the pool's membership and ordering. Failures are logged and repaired by
// a later tick, never fatal: the pool state is a probing heuristic, and
// crocochrome's create-if-free remains the allocation gate.
func (p *Pool) syncOnce(ctx context.Context) {
	addrs, err := p.cfg.resolveFleet()
	if err != nil {
		// A resolution blip must not drop known instances: reconcile the
		// current membership instead.
		p.metrics.syncs.WithLabelValues("error").Inc()
		p.logger.Warn().Err(err).Msg("resolving browser pool fleet, keeping current membership")
		addrs = p.instanceURLs()
	} else {
		p.metrics.syncs.WithLabelValues("ok").Inc()
	}

	// Observe every instance concurrently, outside the mutex.
	type observation struct {
		baseURL string
		free    bool
	}

	observations := make([]observation, len(addrs))

	var wg sync.WaitGroup
	for i, addr := range addrs {
		wg.Go(func() {
			free, err := p.observeInstance(ctx, addr)
			if err != nil {
				// Pessimistic: an unobservable instance is deprioritized, and
				// probing corrects the guess if it is actually free.
				p.logger.Debug().Err(err).Str("instance", addr).Msg("observing browser instance")
				free = false
			}
			observations[i] = observation{baseURL: addr, free: free}
		})
	}

	wg.Wait()

	for _, o := range observations {
		p.applyObservation(o.baseURL, o.free)
	}

	resolved := make(map[string]bool, len(addrs))
	for _, addr := range addrs {
		resolved[addr] = true
	}
	p.prune(resolved)
}

// observeInstance reports whether an instance is free, i.e. has no active
// sessions.
func (p *Pool) observeInstance(ctx context.Context, baseURL string) (bool, error) {
	endpoint, err := url.JoinPath(baseURL, "/sessions")
	if err != nil {
		return false, fmt.Errorf("building sessions URL: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, syncRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, fmt.Errorf("building sessions request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("requesting sessions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("instance responded %d", resp.StatusCode)
	}

	var sessions []string
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return false, fmt.Errorf("decoding sessions: %w", err)
	}

	return len(sessions) == 0, nil
}

// applyObservation records an instance's observed busy/free state: unknown
// instances join the pool, known ones are reordered (free floats to the
// front, busy sinks to the back). Instances busy with a local claim are left
// untouched: the in-flight Acquire or the pending release owns their
// transitions, and the observation may predate the claim.
func (p *Pool) applyObservation(baseURL string, free bool) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	el, found := p.elements[baseURL]
	if !found {
		el = p.order.PushFront(&instance{baseURL: baseURL})
		p.elements[baseURL] = el
	}

	// Only a pre-existing instance can be busy (a just-inserted one never is):
	// it is claimed by this agent, and the claim holder owns its transitions.
	// The busy flag is re-checked here, under the mutex, because the
	// observation was taken outside it and may predate the claim.
	if el.Value.(*instance).busy {
		return
	}

	if free {
		p.order.MoveToFront(el)
	} else {
		p.order.MoveToBack(el)
	}
}

// prune removes instances that are no longer part of the resolved fleet.
// Busy instances are kept until released — a scale-down must not yank state
// out from under a running check — and removed by a later tick.
func (p *Pool) prune(resolved map[string]bool) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for baseURL, el := range p.elements {
		// Busy instances are kept in expectation of their pending release:
		// a pod dropped from discovery may still be draining our session.
		if resolved[baseURL] || el.Value.(*instance).busy {
			continue
		}
		p.order.Remove(el)
		delete(p.elements, baseURL)
	}
}

// claimedCount returns how many instances are claimed by this agent, and the
// total number of instances. It backs the instances gauges.
func (p *Pool) claimedCount() (busy, total int) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for el := p.order.Front(); el != nil; el = el.Next() {
		if el.Value.(*instance).busy {
			busy++
		}
	}
	return busy, p.order.Len()
}

// instanceURLs returns the base URLs of the current instances.
func (p *Pool) instanceURLs() []string {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	addrs := make([]string, 0, len(p.elements))
	for baseURL := range p.elements {
		addrs = append(addrs, baseURL)
	}
	return addrs
}

// moveToBack moves inst's element to the back of the list. The caller
// must hold p.mtx. It is a no-op if the instance has been removed from the
// pool meanwhile.
func (p *Pool) moveToBack(inst *instance) {
	if el, found := p.elements[inst.baseURL]; found && el.Value.(*instance) == inst {
		p.order.MoveToBack(el)
	}
}

// moveToFront is the counterpart of moveToBackLocked.
// The caller must hold p.mtx.
func (p *Pool) moveToFront(inst *instance) {
	if el, found := p.elements[inst.baseURL]; found && el.Value.(*instance) == inst {
		p.order.MoveToFront(el)
	}
}

// registerMetrics builds and registers the pool's metrics. The instances
// gauges report this agent's use of the pool (busy = claimed by this agent);
// the fleet-wide busy ratio is instead derived from each crocochrome
// instance's own sm_crocochrome_session_active metric.
func registerMetrics(registerer prometheus.Registerer, p *Pool) metrics {
	counterOpts := func(name, help string) prometheus.CounterOpts {
		return prometheus.CounterOpts{
			Namespace: metricNamespace,
			Subsystem: metricSubsystem,
			Name:      name,
			Help:      help,
		}
	}
	instancesOpts := func(state string) prometheus.GaugeOpts {
		return prometheus.GaugeOpts{
			Namespace:   metricNamespace,
			Subsystem:   metricSubsystem,
			Name:        "instances",
			Help:        "Number of pool instances by state, where busy means claimed by this agent.",
			ConstLabels: prometheus.Labels{"state": state},
		}
	}

	m := metrics{
		acquires: prometheus.NewCounterVec(
			counterOpts("acquires_total", "Total browser session acquisitions by result. \"exhausted\" means no instance became free within the acquire budget: the check failed for lack of browser capacity."),
			[]string{"result"},
		),
		probes: prometheus.NewCounterVec(
			counterOpts("probes_total", "Total session acquire attempts on individual instances by result. The probes/acquires ratio reflects pool contention."),
			[]string{"result"},
		),
		releases: prometheus.NewCounterVec(
			counterOpts("releases_total", "Total browser session releases by result. Sessions whose release failed are reclaimed by the instance's session timeout."),
			[]string{"result"},
		),
		syncs: prometheus.NewCounterVec(
			counterOpts("syncs_total", "Total pool sync ticks by result. A sync errors when fleet resolution fails; it then reconciles the known instances only."),
			[]string{"result"},
		),
		acquireDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Subsystem: metricSubsystem,
			Name:      "acquire_duration_seconds",
			Help:      "Time spent acquiring a browser session, including probe retries and backoff, for both successful and failed acquisitions.",
			// The acquire budget is capped at 30s (see browserAcquireTimeoutCap
			// in k6runner); deadline-hit acquires observe slightly past it, so
			// the last bucket sits above 30 to catch them.
			Buckets: []float64{0.25, 0.5, 1, 2, 4, 8, 10, 12, 16, 20, 24, 28, 30, 32},
		}),
		instancesBusy: prometheus.NewGaugeFunc(instancesOpts("busy"), func() float64 {
			busy, _ := p.claimedCount()
			return float64(busy)
		}),
		instancesFree: prometheus.NewGaugeFunc(instancesOpts("free"), func() float64 {
			busy, total := p.claimedCount()
			return float64(total - busy)
		}),
	}

	registerer.MustRegister(
		m.acquires,
		m.probes,
		m.releases,
		m.syncs,
		m.acquireDuration,
		m.instancesBusy,
		m.instancesFree,
	)

	return m
}
