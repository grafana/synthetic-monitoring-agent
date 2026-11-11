package adhoc

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// These tests talk to a real gRPC server over an in-memory connection,
// so that stream aborts, reconnections and RPCs the server does not
// implement behave like they do in production.

// transportServer is a minimal API: it hands out ad-hoc requests and
// collects the results the probe reports.
type transportServer struct {
	sm.UnimplementedAdHocChecksServer

	requests []*sm.AdHocRequest

	// onResult, if set, is called for every result received. Returning
	// an error rejects the result.
	onResult func(*sm.AdHocResult) error

	// abortFirstStream, if set, causes the first GetAdHocChecks stream
	// to fail with Unavailable once the channel is closed. Requests are
	// only ever sent on the first stream.
	abortFirstStream chan struct{}

	connects atomic.Int32

	mu      sync.Mutex
	results map[string]*sm.AdHocResult

	// reported receives every result, in the order they arrive.
	reported chan *sm.AdHocResult
}

func newTransportServer(requests ...*sm.AdHocRequest) *transportServer {
	return &transportServer{
		requests: requests,
		results:  make(map[string]*sm.AdHocResult),
		reported: make(chan *sm.AdHocResult, 256),
	}
}

func (s *transportServer) RegisterProbe(context.Context, *sm.ProbeInfo) (*sm.RegisterProbeResult, error) {
	return &sm.RegisterProbeResult{
		Probe:  sm.Probe{Id: 7, Name: "transportProbe"},
		Status: sm.Status{Code: sm.StatusCode_OK},
	}, nil
}

func (s *transportServer) GetAdHocChecks(_ *sm.Void, stream sm.AdHocChecks_GetAdHocChecksServer) error {
	first := s.connects.Add(1) == 1

	if first {
		for _, r := range s.requests {
			if err := stream.Send(r); err != nil {
				return err
			}
		}
	}

	if first && s.abortFirstStream != nil {
		select {
		case <-s.abortFirstStream:
			return status.Error(codes.Unavailable, "dropped by test")
		case <-stream.Context().Done():
			return nil
		}
	}

	<-stream.Context().Done()

	return nil
}

func (s *transportServer) ReportAdHocResult(_ context.Context, result *sm.AdHocResult) (*sm.Void, error) {
	s.mu.Lock()
	s.results[result.ExecutionId] = result
	s.mu.Unlock()

	s.reported <- result

	if s.onResult != nil {
		if err := s.onResult(result); err != nil {
			return nil, err
		}
	}

	return &sm.Void{}, nil
}

func (s *transportServer) resultFor(id string) *sm.AdHocResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.results[id]
}

func (s *transportServer) waitReported(t *testing.T) *sm.AdHocResult {
	t.Helper()

	select {
	case r := <-s.reported:
		return r
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for a result to be reported")
		return nil
	}
}

// startTransport serves srv over an in-memory listener and returns a
// client connection to it.
func startTransport(t *testing.T, srv sm.AdHocChecksServer) *grpc.ClientConn {
	t.Helper()

	lis := bufconn.Listen(1 << 20)

	s := grpc.NewServer()
	sm.RegisterAdHocChecksServer(s, srv)

	go func() { _ = s.Serve(lis) }()

	t.Cleanup(s.Stop)

	cc, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() { _ = cc.Close() })

	return cc
}

// startTransportHandler runs a Handler against srv until the test ends.
func startTransportHandler(t *testing.T, srv sm.AdHocChecksServer, factory func(context.Context, *sm.AdHocRequest) (*runner, error)) <-chan pusher.Payload {
	t.Helper()

	features := feature.NewCollection()
	require.NoError(t, features.Set("adhoc"))

	publishCh := make(chan pusher.Payload, 256)

	h, err := NewHandler(HandlerOpts{
		Conn:           startTransport(t, srv),
		Logger:         zerolog.New(io.Discard),
		Backoff:        testBackoff{},
		Publisher:      channelPublisher(publishCh),
		TenantCh:       make(chan sm.Tenant, 1),
		PromRegisterer: prometheus.NewPedanticRegistry(),
		Features:       features,
		runnerFactory:  factory,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)

		_ = h.Run(ctx)
	}()

	t.Cleanup(func() {
		cancel()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("handler did not stop")
		}
	})

	return publishCh
}

func transportRequest(id string) *sm.AdHocRequest {
	return &sm.AdHocRequest{
		AdHocCheck: sm.AdHocCheck{
			Id:       id,
			TenantId: 1,
			Target:   "testTarget",
			Timeout:  1000,
			Settings: sm.CheckSettings{Ping: &sm.PingSettings{}},
		},
	}
}

func proberFactory(p func() proberLike) func(context.Context, *sm.AdHocRequest) (*runner, error) {
	return func(_ context.Context, req *sm.AdHocRequest) (*runner, error) {
		return &runner{
			logger:  zerolog.New(io.Discard),
			prober:  p(),
			id:      req.AdHocCheck.Id,
			target:  req.AdHocCheck.Target,
			probe:   "transportProbe",
			timeout: 10 * time.Second,
		}, nil
	}
}

// proberLike is the subset of prober.Prober the tests need.
type proberLike interface {
	Name() string
	Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger, executionID string) (bool, float64)
}

// gatedProber logs, waits for the gate to open, and logs again.
type gatedProber struct{ gate <-chan struct{} }

func (gatedProber) Name() string { return "gated" }

func (p gatedProber) Probe(ctx context.Context, _ string, _ *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	_ = l.Log("msg", "first")

	select {
	case <-p.gate:
	case <-ctx.Done():
		return false, 0
	}

	_ = l.Log("msg", "second")

	return true, 1
}

func waitPublished(t *testing.T, publishCh <-chan pusher.Payload) pusher.Payload {
	t.Helper()

	select {
	case p := <-publishCh:
		return p
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the Loki publish")
		return nil
	}
}

// Many executions at once: each result must reach the API under its own
// execution id.
func TestTransportConcurrentExecutions(t *testing.T) {
	const n = 50

	requests := make([]*sm.AdHocRequest, 0, n)
	for i := range n {
		requests = append(requests, transportRequest(fmt.Sprintf("exec-%d", i)))
	}

	srv := newTransportServer(requests...)

	publishCh := startTransportHandler(t, srv,
		proberFactory(func() proberLike { return &testProber{zerolog.New(io.Discard)} }))

	for range n {
		srv.waitReported(t)
		waitPublished(t, publishCh)
	}

	for _, req := range requests {
		id := req.AdHocCheck.Id

		result := srv.resultFor(id)
		require.NotNil(t, result, id)
		require.Equal(t, id, result.ExecutionId)
		require.Equal(t, int64(7), result.ProbeId)
		require.Equal(t, sm.AdHocResult_OK, result.Status, id)
	}
}

// The API rejects the result. The check result in Loki is not affected,
// and the probe does not retry or drop the request stream.
func TestTransportResultRejected(t *testing.T) {
	srv := newTransportServer(transportRequest("rejected"))
	srv.onResult = func(*sm.AdHocResult) error {
		return status.Error(codes.Internal, "rejected by test")
	}

	gate := make(chan struct{})
	close(gate)

	publishCh := startTransportHandler(t, srv,
		proberFactory(func() proberLike { return gatedProber{gate: gate} }))

	srv.waitReported(t)

	line := waitPublished(t, publishCh).Streams()[0].Entries[0].Line
	require.Contains(t, line, "first")
	require.Contains(t, line, "second")

	// Give a (wrong) retry or reconnect a chance to happen.
	time.Sleep(200 * time.Millisecond)

	require.Empty(t, srv.reported, "the result must be reported once")
	require.Equal(t, int32(1), srv.connects.Load(), "a rejected result must not drop the request stream")
}

// The connection that delivers requests drops and is re-established
// while a check is running. Reporting is independent of it: the result
// must still arrive.
func TestTransportReconnectWhileRunning(t *testing.T) {
	srv := newTransportServer(transportRequest("reconnect"))
	srv.abortFirstStream = make(chan struct{})

	gate := make(chan struct{})

	publishCh := startTransportHandler(t, srv,
		proberFactory(func() proberLike { return gatedProber{gate: gate} }))

	close(srv.abortFirstStream)

	require.Eventually(t, func() bool { return srv.connects.Load() >= 2 }, 10*time.Second, 10*time.Millisecond,
		"the handler should reconnect")

	close(gate)

	result := srv.waitReported(t)
	waitPublished(t, publishCh)

	require.Equal(t, "reconnect", result.ExecutionId)
	require.Equal(t, sm.AdHocResult_OK, result.Status)
}

// oldAPIServer is an API that predates ReportAdHocResult: it embeds the
// generated Unimplemented server, so gRPC itself answers Unimplemented.
type oldAPIServer struct {
	sm.UnimplementedAdHocChecksServer

	request  *sm.AdHocRequest
	connects atomic.Int32
}

func (s *oldAPIServer) RegisterProbe(context.Context, *sm.ProbeInfo) (*sm.RegisterProbeResult, error) {
	return &sm.RegisterProbeResult{
		Probe:  sm.Probe{Id: 7, Name: "transportProbe"},
		Status: sm.Status{Code: sm.StatusCode_OK},
	}, nil
}

func (s *oldAPIServer) GetAdHocChecks(_ *sm.Void, stream sm.AdHocChecks_GetAdHocChecksServer) error {
	if s.connects.Add(1) == 1 {
		if err := stream.Send(s.request); err != nil {
			return err
		}
	}

	<-stream.Context().Done()

	return nil
}

// A new probe talking to an API that does not implement the RPC, over a
// real connection: the ad-hoc check still runs and reaches Loki, and the
// refusal does not disturb the stream of requests.
func TestTransportOldAPIWithoutReportRPC(t *testing.T) {
	srv := &oldAPIServer{request: transportRequest("old-api")}

	publishCh := startTransportHandler(t, srv,
		proberFactory(func() proberLike { return &testProber{zerolog.New(io.Discard)} }))

	payload := waitPublished(t, publishCh)
	require.Contains(t, payload.Streams()[0].Entries[0].Line, "ad-hoc check done")

	// Give a (wrong) reconnect a chance to happen.
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, int32(1), srv.connects.Load(), "Unimplemented on the result RPC must not drop the request stream")
}
