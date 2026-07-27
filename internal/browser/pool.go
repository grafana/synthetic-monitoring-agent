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
	"net/http"
	"net/url"
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
	defaultSyncInterval = 15 * time.Second

	// acquireAttemptTimeout bounds a single acquire request to an instance,
	// which launches a Chromium process before responding.
	acquireAttemptTimeout = 10 * time.Second
	// releaseTimeout bounds the session delete request on release.
	releaseTimeout = 10 * time.Second

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
}

// Pool tracks a fleet of single-session crocochrome instances and allocates
// browser sessions from it. A single Pool is shared by all concurrent check
// executions; all state updates are serialized under its mutex.
type Pool struct {
	cfg    Config
	client *http.Client
	logger zerolog.Logger

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

// sessionInfo mirrors the relevant parts of crocochrome's session creation
// response.
type sessionInfo struct {
	ID              string `json:"id"`
	ChromiumVersion struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	} `json:"chromiumVersion"`
}

// New creates a Pool from cfg. registerer will register the pool's metrics
// when they are added; it is accepted now for signature stability.
func New(cfg Config, registerer prometheus.Registerer) (*Pool, error) {
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

	return &Pool{
		cfg:      cfg,
		client:   cfg.HTTPClient,
		logger:   cfg.Logger,
		order:    list.New(),
		elements: make(map[string]*list.Element),
	}, nil
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
		return "", "", fmt.Errorf("requesting session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 409 (busy) and 503 (draining) are expected; anything else is only a
		// log distinction, the instance is skipped all the same.
		return "", "", fmt.Errorf("instance responded %d", resp.StatusCode)
	}

	var si sessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&si); err != nil {
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
		return "", "", errors.New("session response missing id or webSocketDebuggerUrl")
	}

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
				p.logger.Warn().Err(err).Str("instance", inst.baseURL).Str("sessionID", sessionID).
					Msg("releasing browser session")
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

// upsertInstance adds an instance at the front of the list if it is not
// already known. Used by the sync loop on membership changes.
func (p *Pool) upsertInstance(baseURL string) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if _, found := p.elements[baseURL]; found {
		return
	}
	p.elements[baseURL] = p.order.PushFront(&instance{baseURL: baseURL})
}

// removeInstance removes an instance from the pool. Busy instances are kept
// until released, and removed by a later sync tick. Used by the sync loop on
// membership changes.
func (p *Pool) removeInstance(baseURL string) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	el, found := p.elements[baseURL]
	if !found || el.Value.(*instance).busy {
		return
	}
	p.order.Remove(el)
	delete(p.elements, baseURL)
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
