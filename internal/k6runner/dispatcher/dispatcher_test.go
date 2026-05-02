package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner/tiermap"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner/version"
)

func mustMapping(t *testing.T) *tiermap.Live {
	t.Helper()
	m, err := tiermap.New([]byte(`
browser:
  default: browser-A
  tenants:
    "1234": browser-B
small:
  default: small
`))
	require.NoError(t, err)
	return tiermap.NewLive(m, nil, zerolog.Nop())
}

func newTestDispatcher(t *testing.T, opts ...func(*Config)) (*Dispatcher, *Metrics, *httptest.Server) {
	t.Helper()
	cfg := Config{
		Hold:          250 * time.Millisecond,
		DequeueHold:   250 * time.Millisecond,
		QueueCapacity: 4,
		Tiers:         []string{"small", "browser-A", "browser-B"},
	}
	for _, f := range opts {
		f(&cfg)
	}
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	d, err := New(cfg, mustMapping(t), metrics, zerolog.Nop())
	require.NoError(t, err)

	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)
	return d, metrics, srv
}

func mkRequest(checkType, tenantID string) k6runner.HTTPRunRequest {
	return k6runner.HTTPRunRequest{
		Script: k6runner.Script{
			Script:   []byte("export default function() {}"),
			Settings: k6runner.Settings{Timeout: 5000},
			CheckInfo: k6runner.CheckInfo{
				Type: checkType,
				Metadata: map[string]any{
					"tenantID": tenantID,
				},
			},
			K6ChannelManifest: "*",
		},
		NotAfter: time.Now().Add(30 * time.Second),
	}
}

// postRun submits a /run request and decodes the response body into a
// [k6runner.RunResponse]. It returns the decoded body, the response
// status code, and the response headers — the *http.Response itself is
// not exposed because callers tend to forget to close its body.
func postRun(t *testing.T, srv *httptest.Server, req k6runner.HTTPRunRequest) (k6runner.RunResponse, int, http.Header) {
	t.Helper()
	body, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := http.Post(srv.URL+"/run", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var out k6runner.RunResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out, resp.StatusCode, resp.Header
}

func TestDispatcher_RoundTripSuccess(t *testing.T) {
	t.Parallel()

	_, _, srv := newTestDispatcher(t)

	// Worker side: pull from /dequeue?tier=small, post a result back.
	workerErr := make(chan error, 1)
	go func() {
		resp, err := http.Post(srv.URL+"/dequeue?tier=small", "", nil)
		if err != nil {
			workerErr <- err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			workerErr <- nil
			return
		}
		var env dequeueEnvelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			workerErr <- err
			return
		}
		// Post the result.
		body, err := json.Marshal(k6runner.RunResponse{
			ErrorCode: "",
			Metrics:   []byte("# HELP\n"),
			Logs:      []byte("level=info msg=ok\n"),
		})
		if err != nil {
			workerErr <- err
			return
		}
		resultResp, err := http.Post(srv.URL+"/result/"+env.JobID, "application/json", bytes.NewReader(body))
		if err != nil {
			workerErr <- err
			return
		}
		_ = resultResp.Body.Close()
		workerErr <- nil
	}()

	out, status, _ := postRun(t, srv, mkRequest("scripted", "1234"))
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "", out.ErrorCode)
	require.Equal(t, "level=info msg=ok\n", string(out.Logs))

	require.NoError(t, <-workerErr)
}

func TestDispatcher_HoldExpiredNoWorker(t *testing.T) {
	t.Parallel()

	_, _, srv := newTestDispatcher(t, func(c *Config) {
		c.Hold = 80 * time.Millisecond
	})

	out, status, hdr := postRun(t, srv, mkRequest("scripted", "1234"))
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Equal(t, k6runner.ErrorCodeDispatchCapacity, out.ErrorCode)
	require.Empty(t, hdr.Get(k6runner.DrainHeader))
}

func TestDispatcher_QueueOverflow(t *testing.T) {
	t.Parallel()

	_, _, srv := newTestDispatcher(t, func(c *Config) {
		c.Hold = 5 * time.Second // long enough that hold won't trip during the test
		c.QueueCapacity = 2
	})

	// Fill the small tier's queue with two requests held by their /run handlers.
	send := func() {
		_, _, _ = postRun(t, srv, mkRequest("scripted", "1234"))
	}
	go send()
	go send()
	// Give the two requests time to enqueue.
	time.Sleep(50 * time.Millisecond)

	// Third request should overflow → 503 dispatch_capacity immediately.
	out, status, _ := postRun(t, srv, mkRequest("scripted", "1234"))
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Equal(t, k6runner.ErrorCodeDispatchCapacity, out.ErrorCode)
}

func TestDispatcher_DrainReturnsQueuedJobs(t *testing.T) {
	t.Parallel()

	d, _, srv := newTestDispatcher(t, func(c *Config) {
		c.Hold = 5 * time.Second
	})

	results := make(chan k6runner.RunResponse, 1)
	headers := make(chan http.Header, 1)
	go func() {
		out, _, hdr := postRun(t, srv, mkRequest("scripted", "1234"))
		headers <- hdr
		results <- out
	}()

	// Wait for the request to be in the queue.
	require.Eventually(t, func() bool {
		return d.queues["small"].depth() > 0
	}, time.Second, 5*time.Millisecond)

	// Drain — queued job should be returned with dispatcher_drain.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	d.Drain(ctx)

	select {
	case out := <-results:
		require.Equal(t, k6runner.ErrorCodeDispatcherDrain, out.ErrorCode)
	case <-time.After(time.Second):
		t.Fatal("/run did not return after drain")
	}

	hdrs := <-headers
	require.Equal(t, "1", hdrs.Get(k6runner.DrainHeader))
}

func TestDispatcher_DrainRejectsNewRun(t *testing.T) {
	t.Parallel()

	d, _, srv := newTestDispatcher(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // already-cancelled ctx; Drain returns near-immediately.
	d.Drain(ctx)

	out, status, hdr := postRun(t, srv, mkRequest("scripted", "1234"))
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Equal(t, k6runner.ErrorCodeDispatcherDrain, out.ErrorCode)
	require.Equal(t, "1", hdr.Get(k6runner.DrainHeader))
}

func TestDispatcher_DequeueOnEmptyTierReturns204(t *testing.T) {
	t.Parallel()

	_, _, srv := newTestDispatcher(t, func(c *Config) {
		c.DequeueHold = 50 * time.Millisecond
	})

	resp, err := http.Post(srv.URL+"/dequeue?tier=small", "", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestDispatcher_DequeueUnknownTierIs400(t *testing.T) {
	t.Parallel()

	_, _, srv := newTestDispatcher(t)

	resp, err := http.Post(srv.URL+"/dequeue?tier=does-not-exist", "", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestDispatcher_TierRoutingByCheckTypeAndTenant(t *testing.T) {
	t.Parallel()

	d, _, srv := newTestDispatcher(t, func(c *Config) {
		c.Hold = 30 * time.Millisecond // expire fast; we only care about which tier it landed on.
	})

	cases := []struct {
		checkType string
		tenantID  string
		wantTier  string
	}{
		{"scripted", "1234", "small"},
		{"multihttp", "9999", "small"},
		{"browser", "1234", "browser-B"},
		{"browser", "9999", "browser-A"},
	}
	for _, tc := range cases {
		// Fire the request; observe queue depth on the expected tier briefly while it's in the queue.
		var wg sync.WaitGroup
		wg.Go(func() {
			_, _, _ = postRun(t, srv, mkRequest(tc.checkType, tc.tenantID))
		})

		require.Eventually(t, func() bool {
			return d.queues[tc.wantTier].depth() > 0
		}, 200*time.Millisecond, 5*time.Millisecond,
			"check %s tenant %s expected to land in tier %s", tc.checkType, tc.tenantID, tc.wantTier)

		wg.Wait()
	}
}

// stubRepository is an in-memory [Repository] for unit tests; it bypasses the on-disk scan that
// [version.Repository] performs.
type stubRepository struct {
	entries     []version.Entry
	entriesErr  error
	resolveFunc func(string) (*version.Entry, error)
}

func (s *stubRepository) Entries() ([]version.Entry, error) {
	return s.entries, s.entriesErr
}

func (s *stubRepository) Resolve(c string) (*version.Entry, error) {
	if s.resolveFunc != nil {
		return s.resolveFunc(c)
	}
	return nil, errors.New("not implemented")
}

func mustVersion(t *testing.T, v string) *semver.Version {
	t.Helper()
	parsed, err := semver.NewVersion(v)
	require.NoError(t, err)
	return parsed
}

func TestDispatcher_VersionsHappyPath(t *testing.T) {
	t.Parallel()

	repo := &stubRepository{
		entries: []version.Entry{
			{Path: "/k6/k6-v1", Version: mustVersion(t, "1.2.3")},
			{Path: "/k6/k6-v0", Version: mustVersion(t, "0.51.0")},
		},
	}
	_, _, srv := newTestDispatcher(t, func(c *Config) { c.Repository = repo })

	resp, err := http.Get(srv.URL + "/versions")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var body struct {
		Versions []struct {
			Path    string `json:"path"`
			Version string `json:"version"`
		} `json:"versions"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Versions, 2)
	require.Equal(t, "/k6/k6-v1", body.Versions[0].Path)
	require.Equal(t, "1.2.3", body.Versions[0].Version)
}

func TestDispatcher_VersionsEmptyRepoReturnsNullArray(t *testing.T) {
	t.Parallel()

	repo := &stubRepository{} // no entries, no error
	_, _, srv := newTestDispatcher(t, func(c *Config) { c.Repository = repo })

	resp, err := http.Get(srv.URL + "/versions")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"versions":null}`, string(body))
}

func TestDispatcher_VersionsScanError(t *testing.T) {
	t.Parallel()

	repo := &stubRepository{entriesErr: errors.New("disk on fire")}
	_, _, srv := newTestDispatcher(t, func(c *Config) { c.Repository = repo })

	resp, err := http.Get(srv.URL + "/versions")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestDispatcher_VersionsNoRepository(t *testing.T) {
	t.Parallel()

	_, _, srv := newTestDispatcher(t) // no Repository

	resp, err := http.Get(srv.URL + "/versions")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestDispatcher_VersionsResolve(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		query      string
		resolve    func(string) (*version.Entry, error)
		wantStatus int
		wantPath   string
	}{
		{
			name:  "happy",
			query: "manifest=^1.0.0",
			resolve: func(c string) (*version.Entry, error) {
				require.Equal(t, "^1.0.0", c)
				return &version.Entry{Path: "/k6/k6-v1", Version: mustVersion(t, "1.2.3")}, nil
			},
			wantStatus: http.StatusOK,
			wantPath:   "/k6/k6-v1",
		},
		{
			name:       "missing manifest",
			query:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "no match",
			query: "manifest=^99.0.0",
			resolve: func(string) (*version.Entry, error) {
				return nil, version.ErrUnsatisfiable
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:  "bad constraint",
			query: "manifest=garbage",
			resolve: func(string) (*version.Entry, error) {
				return nil, errors.Join(errors.New("parse"), version.ErrInvalidConstraint)
			},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:  "scan error",
			query: "manifest=*",
			resolve: func(string) (*version.Entry, error) {
				return nil, errors.New("disk on fire")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &stubRepository{resolveFunc: tc.resolve}
			_, _, srv := newTestDispatcher(t, func(c *Config) { c.Repository = repo })

			url := srv.URL + "/versions/resolve"
			if tc.query != "" {
				url += "?" + tc.query
			}
			resp, err := http.Get(url)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, tc.wantStatus, resp.StatusCode)

			if tc.wantStatus == http.StatusOK {
				var got struct {
					Path    string `json:"path"`
					Version string `json:"version"`
				}
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
				require.Equal(t, tc.wantPath, got.Path)
			}
		})
	}
}

func TestDispatcher_RunRewritesEmptyManifest(t *testing.T) {
	t.Parallel()

	_, _, srv := newTestDispatcher(t, func(c *Config) {
		c.Hold = 5 * time.Second
		c.DequeueHold = 500 * time.Millisecond
	})

	req := mkRequest("scripted", "1234")
	req.K6ChannelManifest = ""

	envelope := runAndDequeue(t, srv, req, "small", k6runner.RunResponse{})
	require.Equal(t, "*", envelope.Request.K6ChannelManifest)
}

func TestDispatcher_RunClampsNotAfter(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		notAfter  time.Time
		wantClamp bool
	}{
		{"far future", time.Now().Add(2 * time.Hour), true},
		{"far past", time.Now().Add(-30 * 24 * time.Hour), true},
		{"reasonable", time.Now().Add(30 * time.Second), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, srv := newTestDispatcher(t, func(c *Config) {
				c.Hold = 5 * time.Second
				c.DequeueHold = 500 * time.Millisecond
			})

			req := mkRequest("scripted", "1234")
			req.NotAfter = tc.notAfter

			envelope := runAndDequeue(t, srv, req, "small", k6runner.RunResponse{})
			now := time.Now()
			if tc.wantClamp {
				// Clamped value is now+timeout+2min. Allow a generous slack window for test scheduling.
				expected := now.Add(time.Duration(req.Settings.Timeout) * time.Millisecond).Add(2 * time.Minute)
				require.WithinDuration(t, expected, envelope.Request.NotAfter, 5*time.Second,
					"NotAfter should be clamped to a near-now+timeout+2min value")
			} else {
				require.WithinDuration(t, tc.notAfter, envelope.Request.NotAfter, time.Millisecond,
					"NotAfter should be left alone for sane values")
			}
		})
	}
}

func TestDispatcher_RunUnroutedTierIs422EmptyBody(t *testing.T) {
	t.Parallel()

	_, metrics, srv := newTestDispatcher(t, func(c *Config) {
		// Replace the queues with one that doesn't include "small" so any tenant routed there is "unrouted".
		c.Tiers = []string{"browser-A", "browser-B"}
	})

	body, err := json.Marshal(mkRequest("scripted", "1234")) // routes to "small", which is not in our tiers
	require.NoError(t, err)

	resp, err := http.Post(srv.URL+"/run", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Empty(t, respBody, "spec requires empty body for the unroutable case")

	require.Equal(t, float64(1),
		testutil.ToFloat64(metrics.MappingErrors.WithLabelValues(tiermap.CauseUnknownTier)))
}

func TestDispatcher_RunStatusCodeMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		errorCode  string
		wantStatus int
	}{
		{k6runner.ErrorCodeNone, http.StatusOK},
		{k6runner.ErrorCodeFailed, http.StatusOK},
		{k6runner.ErrorCodeAborted, http.StatusOK},
		{k6runner.ErrorCodeTimeout, http.StatusRequestTimeout},
		{k6runner.ErrorCodeKilled, http.StatusUnprocessableEntity},
		{k6runner.ErrorCodeUnsupportedVersion, http.StatusUnprocessableEntity},
		{k6runner.ErrorCodeBadVersion, http.StatusUnprocessableEntity},
		{k6runner.ErrorCodeBrowser, http.StatusServiceUnavailable},
		{k6runner.ErrorCodeUnknown, http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.errorCode, func(t *testing.T) {
			t.Parallel()
			_, _, srv := newTestDispatcher(t)

			// Worker delivers a synthetic result with the given errorCode.
			workerErr := make(chan error, 1)
			go func() {
				resp, err := http.Post(srv.URL+"/dequeue?tier=small", "", nil)
				if err != nil {
					workerErr <- err
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					workerErr <- errors.New("unexpected dequeue status")
					return
				}
				var env dequeueEnvelope
				if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
					workerErr <- err
					return
				}
				body, _ := json.Marshal(k6runner.RunResponse{
					ErrorCode: tc.errorCode,
					Error:     "synthetic",
					Metrics:   []byte{},
					Logs:      []byte{},
				})
				resultResp, err := http.Post(srv.URL+"/result/"+env.JobID, "application/json", bytes.NewReader(body))
				if err != nil {
					workerErr <- err
					return
				}
				_ = resultResp.Body.Close()
				workerErr <- nil
			}()

			out, status, _ := postRun(t, srv, mkRequest("scripted", "1234"))
			require.Equal(t, tc.wantStatus, status, "errorCode %q", tc.errorCode)
			require.Equal(t, tc.errorCode, out.ErrorCode)
			require.NoError(t, <-workerErr)
		})
	}
}

func TestDispatcher_RunLogsXRequestID(t *testing.T) {
	t.Parallel()

	// We don't have a structured way to capture the dispatcher's log output here without rewiring it.
	// The behaviour we care about (no panic / no rejection on the header) is exercised by simply
	// sending a request with the header through the unrouted-tier path so it logs the warn line
	// that includes clientRequestID.
	_, _, srv := newTestDispatcher(t, func(c *Config) {
		c.Tiers = []string{"browser-A", "browser-B"}
	})

	body, err := json.Marshal(mkRequest("scripted", "1234"))
	require.NoError(t, err)
	httpReq, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/run",
		bytes.NewReader(body))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Request-Id", "my-correlation-id")

	resp, err := http.DefaultClient.Do(httpReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

// dequeueOne pulls a single job off the named tier and decodes the envelope, failing the test if
// no job arrives within DequeueHold.
func dequeueOne(t *testing.T, srv *httptest.Server, tier string) dequeueEnvelope {
	t.Helper()
	resp, err := http.Post(srv.URL+"/dequeue?tier="+tier, "", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "expected a job on /dequeue?tier=%s", tier)

	var env dequeueEnvelope
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	return env
}

// runAndDequeue submits req via /run in a goroutine, dequeues from the named tier, and posts the
// supplied synthetic result back so /run can return cleanly. It returns the dequeue envelope so
// callers can assert on what the dispatcher enqueued (e.g. after rewrites).
func runAndDequeue(t *testing.T, srv *httptest.Server, req k6runner.HTTPRunRequest, tier string,
	result k6runner.RunResponse) dequeueEnvelope {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = postRun(t, srv, req)
	}()

	envelope := dequeueOne(t, srv, tier)

	body, err := json.Marshal(result)
	require.NoError(t, err)
	resp, err := http.Post(srv.URL+"/result/"+envelope.JobID, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	_ = resp.Body.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("/run handler did not return after result delivery")
	}
	return envelope
}
