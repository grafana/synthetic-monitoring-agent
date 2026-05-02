package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner/worker"
)

// TestEndToEnd_DispatcherWorker exercises the cmd-level wiring (mux, readyness, config plumbing) against a stub
// runner. It does not start an actual k6 process; the goal is to confirm the binary's wire-up of the pure packages
// produces a working dispatcher + worker pair.
func TestEndToEnd_DispatcherWorker(t *testing.T) {
	t.Parallel()

	dispatcherAddr := freePort(t)
	workerAdminAddr := freePort(t)

	dispatcherCfg := &runConfig{
		Role:          roleDispatcher,
		ListenAddr:    dispatcherAddr,
		Hold:          2 * time.Second,
		DequeueHold:   500 * time.Millisecond,
		QueueCapacity: 16,
		Tiers:         stringList{"small"},
	}
	dispatcherReg := prometheus.NewRegistry()
	require.NoError(t, registerMetrics(dispatcherReg))
	dispatcherReady := newReadynessHandler()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	g, gctx := errgroup.WithContext(ctx)

	require.NoError(t, runDispatcher(gctx, g, dispatcherCfg, zerolog.Nop(), dispatcherReg, dispatcherReady))

	// Wire a worker pointed at the dispatcher with a stub runner that returns immediately. We bypass runWorker's
	// k6runner.New to avoid the k6 binary discovery path during a pure unit test.
	stub := &stubRunner{response: &k6runner.RunResponse{Logs: []byte("ok\n")}}
	wMetrics := worker.NewMetrics(prometheus.NewRegistry())
	w, err := worker.New(worker.Config{
		DispatcherURL: "http://" + dispatcherAddr,
		Tier:          "small",
		PollTimeout:   1 * time.Second,
		ResultTimeout: 1 * time.Second,
	}, worker.LocalExecutor{Runner: stub, Logger: zerolog.Nop()}, wMetrics, zerolog.Nop())
	require.NoError(t, err)
	g.Go(func() error { return w.Run(gctx) })

	// Wait for dispatcher to be ready.
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + dispatcherAddr + "/ready")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 3*time.Second, 20*time.Millisecond)

	// Submit a /run request as the agent would.
	req := k6runner.HTTPRunRequest{
		Script: k6runner.Script{
			Script:   []byte("export default function() {}"),
			Settings: k6runner.Settings{Timeout: 5000},
			CheckInfo: k6runner.CheckInfo{
				Type:     "scripted",
				Metadata: map[string]any{"tenantID": "1"},
			},
			K6ChannelManifest: "*",
		},
		NotAfter: time.Now().Add(10 * time.Second),
	}
	body, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := http.Post("http://"+dispatcherAddr+"/run", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	var runResp k6runner.RunResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&runResp))
	require.Equal(t, "", runResp.ErrorCode)
	require.Equal(t, "ok\n", string(runResp.Logs))

	cancel()
	_ = g.Wait()
	_ = workerAdminAddr // not used; kept reserved for future tests of admin endpoints.
}

// stubRunner is just enough of [k6runner.Runner] to feed [worker.LocalExecutor].
type stubRunner struct {
	response *k6runner.RunResponse
}

func (s *stubRunner) WithLogger(_ *zerolog.Logger) k6runner.Runner { return s }
func (s *stubRunner) Run(_ context.Context, _ k6runner.Script, _ k6runner.SecretStore) (*k6runner.RunResponse, error) {
	return s.response, nil
}

func (s *stubRunner) Versions(_ context.Context) <-chan []string {
	ch := make(chan []string)
	close(ch)
	return ch
}

// freePort asks the OS for an ephemeral port and returns "127.0.0.1:<port>". The port is released before returning;
// callers race a small window between this function and bind.
func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	return addr
}
