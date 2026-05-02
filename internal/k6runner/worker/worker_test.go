package worker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
)

type stubExecutor struct {
	calls    atomic.Int64
	response *k6runner.RunResponse
	err      error
	delay    time.Duration
}

func (e *stubExecutor) Run(ctx context.Context, _ k6runner.Script, _ k6runner.SecretStore) (*k6runner.RunResponse, error) {
	e.calls.Add(1)
	if e.delay > 0 {
		t := time.NewTimer(e.delay)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-t.C:
		}
	}
	return e.response, e.err
}

// stubDispatcher is the minimum surface the worker talks to.
type stubDispatcher struct {
	mu        chan struct{} // serialise queue access; size 1 for simplicity
	queue     []dequeueEnvelope
	delivered chan deliveredResult
}

type deliveredResult struct {
	jobID    string
	response k6runner.RunResponse
}

func newStubDispatcher() *stubDispatcher {
	return &stubDispatcher{
		mu:        make(chan struct{}, 1),
		delivered: make(chan deliveredResult, 16),
	}
}

func (s *stubDispatcher) push(env dequeueEnvelope) {
	s.mu <- struct{}{}
	s.queue = append(s.queue, env)
	<-s.mu
}

func (s *stubDispatcher) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /dequeue", func(w http.ResponseWriter, _ *http.Request) {
		s.mu <- struct{}{}
		defer func() { <-s.mu }()
		if len(s.queue) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next := s.queue[0]
		s.queue = s.queue[1:]
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(next)
	})
	mux.HandleFunc("POST /result/{id}", func(w http.ResponseWriter, r *http.Request) {
		var resp k6runner.RunResponse
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &resp)
		s.delivered <- deliveredResult{jobID: r.PathValue("id"), response: resp}
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func TestWorker_PullExecuteReport(t *testing.T) {
	t.Parallel()

	stub := newStubDispatcher()
	srv := httptest.NewServer(stub.handler(t))
	defer srv.Close()

	stub.push(dequeueEnvelope{
		JobID: "job-1",
		Tier:  "small",
		Request: k6runner.HTTPRunRequest{
			Script: k6runner.Script{
				Settings:  k6runner.Settings{Timeout: 5000},
				CheckInfo: k6runner.CheckInfo{Type: "scripted"},
			},
			NotAfter: time.Now().Add(30 * time.Second),
		},
	})

	exec := &stubExecutor{response: &k6runner.RunResponse{Logs: []byte("level=info ok\n")}}
	reg := prometheus.NewRegistry()
	w, err := New(Config{
		DispatcherURL: srv.URL,
		Tier:          "small",
		PollTimeout:   500 * time.Millisecond,
	}, exec, NewMetrics(reg), zerolog.Nop())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	doneCh := make(chan error, 1)
	go func() { doneCh <- w.Run(ctx) }()

	select {
	case got := <-stub.delivered:
		require.Equal(t, "job-1", got.jobID)
		require.Equal(t, "level=info ok\n", string(got.response.Logs))
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not deliver result in time")
	}
	require.Equal(t, int64(1), exec.calls.Load())

	cancel()
	<-doneCh
}

func TestWorker_ExecutorErrorMapsToWorkerCrash(t *testing.T) {
	t.Parallel()

	stub := newStubDispatcher()
	srv := httptest.NewServer(stub.handler(t))
	defer srv.Close()

	stub.push(dequeueEnvelope{
		JobID: "job-x",
		Tier:  "small",
		Request: k6runner.HTTPRunRequest{
			Script: k6runner.Script{
				Settings:  k6runner.Settings{Timeout: 5000},
				CheckInfo: k6runner.CheckInfo{Type: "scripted"},
			},
			NotAfter: time.Now().Add(30 * time.Second),
		},
	})

	exec := &stubExecutor{err: errFakeBoom}

	reg := prometheus.NewRegistry()
	w, err := New(Config{DispatcherURL: srv.URL, Tier: "small", PollTimeout: 500 * time.Millisecond},
		exec, NewMetrics(reg), zerolog.Nop())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	doneCh := make(chan error, 1)
	go func() { doneCh <- w.Run(ctx) }()

	select {
	case got := <-stub.delivered:
		require.Equal(t, k6runner.ErrorCodeWorkerCrashMidScript, got.response.ErrorCode)
		require.Contains(t, got.response.Error, "boom")
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not deliver result in time")
	}

	cancel()
	<-doneCh
}

var errFakeBoom = stubError("boom")

type stubError string

func (e stubError) Error() string { return string(e) }

func TestWorker_NewRejectsBadConfig(t *testing.T) {
	t.Parallel()

	exec := &stubExecutor{}
	_, err := New(Config{DispatcherURL: "", Tier: "small"}, exec, nil, zerolog.Nop())
	require.Error(t, err)

	_, err = New(Config{DispatcherURL: "http://x", Tier: ""}, exec, nil, zerolog.Nop())
	require.Error(t, err)

	_, err = New(Config{DispatcherURL: "http://x", Tier: "small"}, nil, nil, zerolog.Nop())
	require.Error(t, err)
}

func TestWorker_HandlesTransientPollFailure(t *testing.T) {
	t.Parallel()

	var failures atomic.Int64
	stub := newStubDispatcher()
	stub.push(dequeueEnvelope{
		JobID: "job-after-fail",
		Tier:  "small",
		Request: k6runner.HTTPRunRequest{
			Script: k6runner.Script{
				Settings:  k6runner.Settings{Timeout: 5000},
				CheckInfo: k6runner.CheckInfo{Type: "scripted"},
			},
			NotAfter: time.Now().Add(30 * time.Second),
		},
	})

	mux := stub.handler(t)
	flaky := http.NewServeMux()
	flaky.HandleFunc("POST /dequeue", func(w http.ResponseWriter, r *http.Request) {
		if failures.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("nope"))
			return
		}
		mux.ServeHTTP(w, r)
	})
	flaky.HandleFunc("POST /result/{id}", mux.ServeHTTP)

	srv := httptest.NewServer(flaky)
	defer srv.Close()

	exec := &stubExecutor{response: &k6runner.RunResponse{}}
	reg := prometheus.NewRegistry()
	w, err := New(Config{DispatcherURL: srv.URL, Tier: "small", PollTimeout: 200 * time.Millisecond},
		exec, NewMetrics(reg), zerolog.Nop())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	doneCh := make(chan error, 1)
	go func() { doneCh <- w.Run(ctx) }()

	select {
	case got := <-stub.delivered:
		require.Equal(t, "job-after-fail", got.jobID)
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not recover from transient failure")
	}

	cancel()
	<-doneCh
}
