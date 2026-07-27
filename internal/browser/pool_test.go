package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
)

// The pool must satisfy the k6 runner's BrowserPool interface, which is
// satisfied structurally: k6runner cannot import this package (this package
// imports it for CheckInfo).
var _ k6runner.BrowserPool = (*Pool)(nil)

func TestNew(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		url       string
		expectErr bool
	}{
		"valid":          {url: "http://crocochrome.pool.svc:8080"},
		"missing scheme": {url: "crocochrome.pool.svc:8080", expectErr: true},
		"missing host":   {url: "http://", expectErr: true},
		"empty":          {url: "", expectErr: true},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			cancel() // keep the sync loop inert.

			_, err := New(ctx, Config{URL: tc.url}, prometheus.NewRegistry())
			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAcquire(t *testing.T) {
	t.Parallel()

	t.Run("single instance", func(t *testing.T) {
		t.Parallel()

		fake := newFakeInstance(t)
		pool := newTestPool(t, fake)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		wsURL, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.Contains(t, wsURL, "/proxy/")
		requireInvariant(t, pool)

		release(t.Context())
		require.Equal(t, 1, fake.deleteCount())
		require.Empty(t, fake.session())
		requireInvariant(t, pool)

		// The instance must be allocatable again.
		wsURL2, release2, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.NotEmpty(t, wsURL2)
		release2(t.Context())
	})

	t.Run("walks past busy", func(t *testing.T) {
		t.Parallel()

		busy := newFakeInstance(t)
		busy.setSession("held-by-someone-else")
		free := newFakeInstance(t)
		pool := newTestPool(t, busy, free)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		wsURL, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.Contains(t, wsURL, hostOf(free))
		require.Equal(t, 1, busy.acquireCount())
		require.Equal(t, 1, free.acquireCount())
		// Both were observed busy (409) or allocated: both moved to the back,
		// so the acquired instance ends up last.
		require.Equal(t, []string{busy.URL(), free.URL()}, poolOrder(pool))
		requireInvariant(t, pool)

		release(t.Context())
		// Released instances return to the front.
		require.Equal(t, []string{free.URL(), busy.URL()}, poolOrder(pool))
		requireInvariant(t, pool)
	})

	t.Run("treats 503 as busy", func(t *testing.T) {
		t.Parallel()

		draining := newFakeInstance(t)
		draining.forceAcquireStatus = http.StatusServiceUnavailable
		free := newFakeInstance(t)
		pool := newTestPool(t, draining, free)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		wsURL, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.Contains(t, wsURL, hostOf(free))
		release(t.Context())
	})

	t.Run("waits for freed instance", func(t *testing.T) {
		t.Parallel()

		a := newFakeInstance(t)
		a.setSession("held")
		b := newFakeInstance(t)
		b.setSession("held")
		pool := newTestPool(t, a, b)

		go func() {
			time.Sleep(300 * time.Millisecond)
			a.setSession("") // freed elsewhere (e.g. session timeout).
		}()

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		wsURL, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.Contains(t, wsURL, hostOf(a))
		release(t.Context())
		requireInvariant(t, pool)
	})

	t.Run("deadline exceeded", func(t *testing.T) {
		t.Parallel()

		fake := newFakeInstance(t)
		fake.setSession("held")
		pool := newTestPool(t, fake)

		ctx, cancel := context.WithTimeout(t.Context(), 700*time.Millisecond)
		defer cancel()

		_, _, err := pool.Acquire(ctx, testCheckInfo())
		require.ErrorIs(t, err, ErrPoolExhausted)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		requireInvariant(t, pool)
	})

	t.Run("empty pool", func(t *testing.T) {
		t.Parallel()

		pool := newTestPool(t)

		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()

		_, _, err := pool.Acquire(ctx, testCheckInfo())
		require.ErrorIs(t, err, ErrPoolExhausted)
	})

	t.Run("undecodable body", func(t *testing.T) {
		t.Parallel()

		bad := newFakeInstance(t)
		bad.badBody = true
		good := newFakeInstance(t)
		pool := newTestPool(t, bad, good)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		wsURL, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.Contains(t, wsURL, hostOf(good))
		require.Equal(t, 1, bad.acquireCount())
		release(t.Context())
	})

	t.Run("missing ws url deletes created session", func(t *testing.T) {
		t.Parallel()

		bad := newFakeInstance(t)
		bad.emptyWSURL = true
		good := newFakeInstance(t)
		pool := newTestPool(t, bad, good)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		wsURL, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.Contains(t, wsURL, hostOf(good))
		// The unusable session on the bad instance was best-effort deleted.
		require.Equal(t, 1, bad.deleteCount())
		require.Empty(t, bad.session())
		release(t.Context())
	})
}

func TestRelease(t *testing.T) {
	t.Parallel()

	t.Run("is idempotent", func(t *testing.T) {
		t.Parallel()

		fake := newFakeInstance(t)
		pool := newTestPool(t, fake)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)

		release(t.Context())
		release(t.Context())
		require.Equal(t, 1, fake.deleteCount())
	})

	t.Run("despite delete failure", func(t *testing.T) {
		t.Parallel()

		fake := newFakeInstance(t)
		fake.failDelete = true
		pool := newTestPool(t, fake)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)

		release(t.Context())
		require.Equal(t, 1, fake.deleteCount())
		require.NotEmpty(t, fake.session()) // the DELETE failed, session leaked...
		requireInvariant(t, pool)

		// ...so the next acquire gets the 409 correction until the instance's
		// session timeout reaps it (simulated here), after which it succeeds.
		shortCtx, shortCancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer shortCancel()
		_, _, err = pool.Acquire(shortCtx, testCheckInfo())
		require.ErrorIs(t, err, ErrPoolExhausted)

		fake.setSession("")
		wsURL, release2, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.NotEmpty(t, wsURL)
		release2(t.Context())
	})
}

func TestAcquireConcurrency(t *testing.T) {
	t.Parallel()

	fakes := []*fakeInstance{newFakeInstance(t), newFakeInstance(t), newFakeInstance(t)}
	pool := newTestPool(t, fakes...)

	const goroutines = 10
	deadline := time.Now().Add(1500 * time.Millisecond)

	var (
		wg        sync.WaitGroup
		successes int64
		mtx       sync.Mutex
	)

	for range goroutines {
		wg.Go(func() {
			for time.Now().Before(deadline) {
				// The timeout is far larger than the test duration: a context
				// expiring mid-probe would leak a session on the instance
				// (reaped by the session timeout in production, but a false
				// positive for the leak assertion below).
				ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
				_, release, err := pool.Acquire(ctx, testCheckInfo())
				if err != nil {
					cancel()
					if errors.Is(err, ErrPoolExhausted) {
						continue
					}
					t.Errorf("unexpected acquire error: %v", err)
					return
				}
				time.Sleep(time.Duration(10+rand.Intn(20)) * time.Millisecond)
				release(t.Context())
				cancel()
				mtx.Lock()
				successes++
				mtx.Unlock()
			}
		})
	}
	wg.Wait()

	mtx.Lock()
	require.Positive(t, successes)
	mtx.Unlock()

	// The pool must never send two concurrent acquire requests to the same
	// instance: the busy claim serializes them locally.
	for i, fake := range fakes {
		require.LessOrEqual(t, fake.maxInFlight(), 1, "instance %d received concurrent acquires", i)
		require.Empty(t, fake.session(), "instance %d leaked a session", i)
	}
	requireInvariant(t, pool)
}

func TestSync(t *testing.T) {
	t.Parallel()

	t.Run("initial population", func(t *testing.T) {
		t.Parallel()

		a := newFakeInstance(t)
		b := newFakeInstance(t)
		pool := newTestPool(t)
		pool.cfg.resolveFleet = staticFleet(a.URL(), b.URL())

		pool.syncOnce(t.Context())
		require.ElementsMatch(t, []string{a.URL(), b.URL()}, poolOrder(pool))
		requireInvariant(t, pool)

		// A second sync must not duplicate instances.
		pool.syncOnce(t.Context())
		require.Len(t, poolOrder(pool), 2)
		requireInvariant(t, pool)
	})

	t.Run("reorders by observed state", func(t *testing.T) {
		t.Parallel()

		// Stale in both directions: a is believed free (front) but holds
		// another agent's session; b is believed busy (back) but is free.
		a := newFakeInstance(t)
		a.setSession("held-by-someone-else")
		b := newFakeInstance(t)
		pool := newTestPool(t, a, b)
		pool.cfg.resolveFleet = staticFleet(a.URL(), b.URL())

		pool.syncOnce(t.Context())
		require.Equal(t, []string{b.URL(), a.URL()}, poolOrder(pool))
		requireInvariant(t, pool)
	})

	t.Run("skips locally busy instances", func(t *testing.T) {
		t.Parallel()

		fake := newFakeInstance(t)
		pool := newTestPool(t, fake)
		pool.cfg.resolveFleet = staticFleet() // instance gone from discovery.

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)

		// While we hold its session, sync must neither remove the instance
		// nor touch its state, even though discovery dropped it.
		pool.syncOnce(t.Context())
		require.Equal(t, []string{fake.URL()}, poolOrder(pool))
		requireInvariant(t, pool)

		// Once released, the next sync prunes it.
		release(t.Context())
		pool.syncOnce(t.Context())
		require.Empty(t, poolOrder(pool))
		requireInvariant(t, pool)
	})

	t.Run("treats list failure as busy", func(t *testing.T) {
		t.Parallel()

		a := newFakeInstance(t)
		a.forceListStatus = http.StatusInternalServerError
		b := newFakeInstance(t)
		pool := newTestPool(t, a, b)
		pool.cfg.resolveFleet = staticFleet(a.URL(), b.URL())

		pool.syncOnce(t.Context())
		// The unobservable instance stays in the pool but sinks to the back.
		require.Equal(t, []string{b.URL(), a.URL()}, poolOrder(pool))
		requireInvariant(t, pool)
	})

	t.Run("keeps membership on resolution error", func(t *testing.T) {
		t.Parallel()

		a := newFakeInstance(t)
		a.setSession("held-by-someone-else")
		b := newFakeInstance(t)
		pool := newTestPool(t, a, b)
		pool.cfg.resolveFleet = func() ([]string, error) {
			return nil, errors.New("dns is down")
		}

		pool.syncOnce(t.Context())
		// Membership is preserved and known instances are still reconciled.
		require.Equal(t, []string{b.URL(), a.URL()}, poolOrder(pool))
		requireInvariant(t, pool)
	})

	t.Run("loop syncs on construction and stops on cancel", func(t *testing.T) {
		t.Parallel()

		a := newFakeInstance(t)
		fleet := &mutableFleet{urls: []string{a.URL()}}

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		pool, err := New(ctx, Config{
			URL:          "http://pool.invalid",
			SyncInterval: 25 * time.Millisecond,
			resolveFleet: fleet.resolve,
		}, prometheus.NewRegistry())
		require.NoError(t, err)

		// The initial sync populates membership without waiting an interval.
		require.Eventually(t, func() bool {
			return len(poolOrder(pool)) == 1
		}, 2*time.Second, 5*time.Millisecond)

		// After cancellation, fleet changes are no longer picked up.
		cancel()
		time.Sleep(50 * time.Millisecond) // let a possibly in-flight tick finish.
		fleet.set()                       // empty fleet: a sync would prune a.
		require.Never(t, func() bool {
			return len(poolOrder(pool)) != 1
		}, 200*time.Millisecond, 25*time.Millisecond)
	})

	t.Run("acquire succeeds without explicit start", func(t *testing.T) {
		t.Parallel()

		a := newFakeInstance(t)

		pool, err := New(t.Context(), Config{
			URL:          "http://pool.invalid",
			resolveFleet: staticFleet(a.URL()),
		}, prometheus.NewRegistry())
		require.NoError(t, err)

		acquireCtx, acquireCancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer acquireCancel()

		wsURL, release, err := pool.Acquire(acquireCtx, testCheckInfo())
		require.NoError(t, err)
		require.Contains(t, wsURL, hostOf(a))
		release(t.Context())
	})
}

func TestInstanceBaseURLs(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		scheme   string
		port     string
		ips      []string
		expected []string
	}{
		"with port": {
			scheme:   "http",
			port:     "8080",
			ips:      []string{"10.0.0.1", "10.0.0.2"},
			expected: []string{"http://10.0.0.1:8080", "http://10.0.0.2:8080"},
		},
		"without port": {
			scheme:   "http",
			ips:      []string{"10.0.0.1"},
			expected: []string{"http://10.0.0.1"},
		},
		"ipv6": {
			scheme:   "http",
			port:     "8080",
			ips:      []string{"fd00::1"},
			expected: []string{"http://[fd00::1]:8080"},
		},
		"ipv6 without port": {
			scheme:   "http",
			ips:      []string{"fd00::1"},
			expected: []string{"http://[fd00::1]"},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, instanceBaseURLs(tc.scheme, tc.port, tc.ips))
		})
	}
}

func TestMetrics(t *testing.T) {
	t.Parallel()

	counter := func(t *testing.T, vec *prometheus.CounterVec, result string) float64 {
		t.Helper()
		c, err := vec.GetMetricWith(prometheus.Labels{"result": result})
		require.NoError(t, err)
		return testutil.ToFloat64(c)
	}

	t.Run("acquire success and release", func(t *testing.T) {
		t.Parallel()

		fake := newFakeInstance(t)
		pool := newTestPool(t, fake)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		release(t.Context())

		require.Equal(t, 1.0, counter(t, pool.metrics.acquires, "success"))
		require.Equal(t, 1.0, counter(t, pool.metrics.probes, "acquired"))
		require.Equal(t, 1.0, counter(t, pool.metrics.releases, "ok"))
		require.Equal(t, uint64(1), histogramSampleCount(t, pool.metrics.acquireDuration))
	})

	t.Run("contention counts probes", func(t *testing.T) {
		t.Parallel()

		busy := newFakeInstance(t)
		busy.setSession("held-by-someone-else")
		draining := newFakeInstance(t)
		draining.forceAcquireStatus = http.StatusServiceUnavailable
		free := newFakeInstance(t)
		pool := newTestPool(t, busy, draining, free)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		release(t.Context())

		require.Equal(t, 1.0, counter(t, pool.metrics.acquires, "success"))
		require.Equal(t, 1.0, counter(t, pool.metrics.probes, "busy"))
		require.Equal(t, 1.0, counter(t, pool.metrics.probes, "draining"))
		require.Equal(t, 1.0, counter(t, pool.metrics.probes, "acquired"))
	})

	t.Run("exhaustion", func(t *testing.T) {
		t.Parallel()

		fake := newFakeInstance(t)
		fake.setSession("held")
		pool := newTestPool(t, fake)

		ctx, cancel := context.WithTimeout(t.Context(), 700*time.Millisecond)
		defer cancel()

		_, _, err := pool.Acquire(ctx, testCheckInfo())
		require.ErrorIs(t, err, ErrPoolExhausted)

		require.Equal(t, 1.0, counter(t, pool.metrics.acquires, "exhausted"))
		require.GreaterOrEqual(t, counter(t, pool.metrics.probes, "busy"), 1.0)
	})

	t.Run("release failure", func(t *testing.T) {
		t.Parallel()

		fake := newFakeInstance(t)
		fake.failDelete = true
		pool := newTestPool(t, fake)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		release(t.Context())

		require.Equal(t, 1.0, counter(t, pool.metrics.releases, "error"))
	})

	t.Run("gauges track claims", func(t *testing.T) {
		t.Parallel()

		a := newFakeInstance(t)
		b := newFakeInstance(t)
		pool := newTestPool(t, a, b)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.Equal(t, 1.0, testutil.ToFloat64(pool.metrics.instancesBusy))
		require.Equal(t, 1.0, testutil.ToFloat64(pool.metrics.instancesFree))

		release(t.Context())
		require.Equal(t, 0.0, testutil.ToFloat64(pool.metrics.instancesBusy))
		require.Equal(t, 2.0, testutil.ToFloat64(pool.metrics.instancesFree))
	})

	t.Run("sync results", func(t *testing.T) {
		t.Parallel()

		pool := newTestPool(t)
		pool.cfg.resolveFleet = staticFleet()
		pool.syncOnce(t.Context())
		require.Equal(t, 1.0, counter(t, pool.metrics.syncs, "ok"))

		pool.cfg.resolveFleet = func() ([]string, error) {
			return nil, errors.New("dns is down")
		}
		pool.syncOnce(t.Context())
		require.Equal(t, 1.0, counter(t, pool.metrics.syncs, "error"))
	})
}

// histogramSampleCount returns the number of observations recorded by a
// histogram.
func histogramSampleCount(t *testing.T, h prometheus.Histogram) uint64 {
	t.Helper()

	var m dto.Metric
	require.NoError(t, h.Write(&m))
	return m.GetHistogram().GetSampleCount()
}

// fakeInstance is a minimal crocochrome instance: single session,
// create-if-free acquire semantics.
type fakeInstance struct {
	srv *httptest.Server

	// Static config, set before the fake receives requests.
	forceAcquireStatus int
	forceListStatus    int
	badBody            bool
	emptyWSURL         bool
	failDelete         bool

	mtx            sync.Mutex
	sessionID      string
	sessionCounter int
	acquires       int
	inFlight       int
	maxInFlightN   int
	deletes        int
}

func newFakeInstance(t *testing.T) *fakeInstance {
	t.Helper()

	f := &fakeInstance{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions", f.handleList)
	mux.HandleFunc("POST /sessions/acquire", f.handleAcquire)
	mux.HandleFunc("DELETE /sessions/{id}", f.handleDelete)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)

	return f
}

func (f *fakeInstance) URL() string { return f.srv.URL }

func (f *fakeInstance) handleAcquire(rw http.ResponseWriter, r *http.Request) {
	f.mtx.Lock()
	f.acquires++
	f.inFlight++
	if f.inFlight > f.maxInFlightN {
		f.maxInFlightN = f.inFlight
	}
	f.mtx.Unlock()

	defer func() {
		f.mtx.Lock()
		f.inFlight--
		f.mtx.Unlock()
	}()

	// Widen the window during which overlapping requests would be observed as
	// concurrent by the in-flight watermark.
	time.Sleep(5 * time.Millisecond)

	f.mtx.Lock()
	defer f.mtx.Unlock()

	if f.forceAcquireStatus != 0 {
		rw.WriteHeader(f.forceAcquireStatus)
		return
	}
	if f.badBody {
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte("{invalid"))
		return
	}
	if f.sessionID != "" {
		rw.WriteHeader(http.StatusConflict)
		return
	}

	f.sessionCounter++
	f.sessionID = fmt.Sprintf("session-%d", f.sessionCounter)
	wsURL := fmt.Sprintf("ws://%s/proxy/%s", r.Host, f.sessionID)
	if f.emptyWSURL {
		wsURL = ""
	}
	rw.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(rw, `{"id":%q,"chromiumVersion":{"webSocketDebuggerUrl":%q}}`, f.sessionID, wsURL)
}

func (f *fakeInstance) handleList(rw http.ResponseWriter, _ *http.Request) {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	if f.forceListStatus != 0 {
		rw.WriteHeader(f.forceListStatus)
		return
	}

	sessions := []string{}
	if f.sessionID != "" {
		sessions = append(sessions, f.sessionID)
	}
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(sessions)
}

func (f *fakeInstance) handleDelete(rw http.ResponseWriter, r *http.Request) {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	f.deletes++
	if f.failDelete {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	if id := r.PathValue("id"); id != "" && id == f.sessionID {
		f.sessionID = ""
		return
	}
	rw.WriteHeader(http.StatusNotFound)
}

func (f *fakeInstance) setSession(id string) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.sessionID = id
}

func (f *fakeInstance) session() string {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return f.sessionID
}

func (f *fakeInstance) acquireCount() int {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return f.acquires
}

func (f *fakeInstance) deleteCount() int {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return f.deletes
}

func (f *fakeInstance) maxInFlight() int {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return f.maxInFlightN
}

// newTestPool creates a Pool seeded with the given fakes, front to back in
// argument order. The sync loop is kept inert (cancelled context) so tests
// stay deterministic; sync scenarios drive syncOnce directly.
func newTestPool(t *testing.T, fakes ...*fakeInstance) *Pool {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	pool, err := New(ctx, Config{URL: "http://pool.invalid"}, prometheus.NewRegistry())
	require.NoError(t, err)

	// applyObservation inserts at the front: seed in reverse so fakes[0] ends
	// up frontmost.
	for i := len(fakes) - 1; i >= 0; i-- {
		pool.applyObservation(fakes[i].URL(), true)
	}

	return pool
}

// staticFleet returns a resolver serving a fixed set of instance base URLs.
func staticFleet(urls ...string) func() ([]string, error) {
	return func() ([]string, error) {
		return urls, nil
	}
}

// mutableFleet is a resolver whose fleet can be swapped concurrently.
type mutableFleet struct {
	mtx  sync.Mutex
	urls []string
}

func (m *mutableFleet) resolve() ([]string, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.urls, nil
}

func (m *mutableFleet) set(urls ...string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.urls = urls
}

func testCheckInfo() k6runner.CheckInfo {
	return k6runner.CheckInfo{
		Type:     "browser",
		Metadata: map[string]any{"id": "123", "tenantID": "1"},
	}
}

func hostOf(f *fakeInstance) string {
	return f.srv.Listener.Addr().String()
}

func poolOrder(p *Pool) []string {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	var out []string
	for el := p.order.Front(); el != nil; el = el.Next() {
		out = append(out, el.Value.(*instance).baseURL)
	}
	return out
}

// requireInvariant asserts that the order list and the elements index are
// consistent: same size, and every list element is indexed under its
// instance's base URL.
func requireInvariant(t *testing.T, p *Pool) {
	t.Helper()

	p.mtx.Lock()
	defer p.mtx.Unlock()

	require.Equal(t, p.order.Len(), len(p.elements))
	for el := p.order.Front(); el != nil; el = el.Next() {
		indexed, found := p.elements[el.Value.(*instance).baseURL]
		require.True(t, found)
		require.Same(t, el, indexed)
	}
}
