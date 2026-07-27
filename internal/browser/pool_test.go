package browser

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
)

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

			_, err := New(Config{URL: tc.url}, prometheus.NewRegistry())
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

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		wsURL, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.Contains(t, wsURL, "/proxy/")
		requireInvariant(t, pool)

		release(context.Background())
		require.Equal(t, 1, fake.deleteCount())
		require.Empty(t, fake.session())
		requireInvariant(t, pool)

		// The instance must be allocatable again.
		wsURL2, release2, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.NotEmpty(t, wsURL2)
		release2(context.Background())
	})

	t.Run("walks past busy", func(t *testing.T) {
		t.Parallel()

		busy := newFakeInstance(t)
		busy.setSession("held-by-someone-else")
		free := newFakeInstance(t)
		pool := newTestPool(t, busy, free)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

		release(context.Background())
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

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		wsURL, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.Contains(t, wsURL, hostOf(free))
		release(context.Background())
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

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		wsURL, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.Contains(t, wsURL, hostOf(a))
		release(context.Background())
		requireInvariant(t, pool)
	})

	t.Run("deadline exceeded", func(t *testing.T) {
		t.Parallel()

		fake := newFakeInstance(t)
		fake.setSession("held")
		pool := newTestPool(t, fake)

		ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
		defer cancel()

		_, _, err := pool.Acquire(ctx, testCheckInfo())
		require.ErrorIs(t, err, ErrPoolExhausted)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		requireInvariant(t, pool)
	})

	t.Run("empty pool", func(t *testing.T) {
		t.Parallel()

		pool := newTestPool(t)

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
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

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		wsURL, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.Contains(t, wsURL, hostOf(good))
		require.Equal(t, 1, bad.acquireCount())
		release(context.Background())
	})

	t.Run("missing ws url deletes created session", func(t *testing.T) {
		t.Parallel()

		bad := newFakeInstance(t)
		bad.emptyWSURL = true
		good := newFakeInstance(t)
		pool := newTestPool(t, bad, good)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		wsURL, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.Contains(t, wsURL, hostOf(good))
		// The unusable session on the bad instance was best-effort deleted.
		require.Equal(t, 1, bad.deleteCount())
		require.Empty(t, bad.session())
		release(context.Background())
	})
}

func TestRelease(t *testing.T) {
	t.Parallel()

	t.Run("is idempotent", func(t *testing.T) {
		t.Parallel()

		fake := newFakeInstance(t)
		pool := newTestPool(t, fake)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)

		release(context.Background())
		release(context.Background())
		require.Equal(t, 1, fake.deleteCount())
	})

	t.Run("despite delete failure", func(t *testing.T) {
		t.Parallel()

		fake := newFakeInstance(t)
		fake.failDelete = true
		pool := newTestPool(t, fake)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, release, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)

		release(context.Background())
		require.Equal(t, 1, fake.deleteCount())
		require.NotEmpty(t, fake.session()) // the DELETE failed, session leaked...
		requireInvariant(t, pool)

		// ...so the next acquire gets the 409 correction until the instance's
		// session timeout reaps it (simulated here), after which it succeeds.
		shortCtx, shortCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer shortCancel()
		_, _, err = pool.Acquire(shortCtx, testCheckInfo())
		require.ErrorIs(t, err, ErrPoolExhausted)

		fake.setSession("")
		wsURL, release2, err := pool.Acquire(ctx, testCheckInfo())
		require.NoError(t, err)
		require.NotEmpty(t, wsURL)
		release2(context.Background())
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
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
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
				release(context.Background())
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

// TestMembership exercises the internal membership mutators (upsertInstance,
// removeInstance) through which the sync loop will grow and shrink the pool:
// upserting an existing instance must not duplicate it, and instances busy
// with an in-flight session must survive removal until released, so that a
// fleet scale-down never yanks state out from under a running check.
func TestMembership(t *testing.T) {
	t.Parallel()

	fake := newFakeInstance(t)
	pool := newTestPool(t, fake)

	// Upsert is idempotent.
	pool.upsertInstance(fake.URL())
	require.Len(t, poolOrder(pool), 1)

	// Busy instances are not removed.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, release, err := pool.Acquire(ctx, testCheckInfo())
	require.NoError(t, err)
	pool.removeInstance(fake.URL())
	require.Len(t, poolOrder(pool), 1)

	// Released instances are.
	release(context.Background())
	pool.removeInstance(fake.URL())
	require.Empty(t, poolOrder(pool))
	requireInvariant(t, pool)
}

// fakeInstance is a minimal crocochrome instance: single session,
// create-if-free acquire semantics.
type fakeInstance struct {
	srv *httptest.Server

	// Static config, set before the fake receives requests.
	forceAcquireStatus int
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
// argument order.
func newTestPool(t *testing.T, fakes ...*fakeInstance) *Pool {
	t.Helper()

	pool, err := New(Config{URL: "http://pool.invalid"}, prometheus.NewRegistry())
	require.NoError(t, err)

	// upsertInstance inserts at the front: seed in reverse so fakes[0] ends up
	// frontmost.
	for i := len(fakes) - 1; i >= 0; i-- {
		pool.upsertInstance(fakes[i].URL())
	}

	return pool
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
