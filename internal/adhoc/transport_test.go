package adhoc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
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
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// These tests talk to a real gRPC server over an in-memory connection,
// so that message size limits, stream aborts and reconnections behave
// like they do in production.

// transportServer is a minimal API: it hands out ad-hoc requests and
// collects what the probe reports.
type transportServer struct {
	sm.UnimplementedAdHocChecksServer

	requests []*sm.AdHocRequest

	// onChunk, if set, is called for every chunk received. Returning an
	// error aborts the stream.
	onChunk func(*sm.AdHocResultChunk) error

	// abortFirstStream, if set, causes the first GetAdHocChecks stream
	// to fail with Unavailable once the channel is closed. Requests are
	// only ever sent on the first stream.
	abortFirstStream chan struct{}

	connects atomic.Int32

	mu     sync.Mutex
	chunks map[string][]*sm.AdHocResultChunk

	// ended receives one value per ReportAdHocResult stream: nil if the
	// probe closed it cleanly, the error otherwise.
	ended chan error
}

func newTransportServer(requests ...*sm.AdHocRequest) *transportServer {
	return &transportServer{
		requests: requests,
		chunks:   make(map[string][]*sm.AdHocResultChunk),
		ended:    make(chan error, 256),
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

func (s *transportServer) ReportAdHocResult(stream sm.AdHocChecks_ReportAdHocResultServer) error {
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			s.ended <- nil

			return stream.SendAndClose(&sm.Void{})
		}

		if err != nil {
			s.ended <- err

			return err
		}

		s.mu.Lock()
		s.chunks[chunk.ExecutionId] = append(s.chunks[chunk.ExecutionId], chunk)
		s.mu.Unlock()

		if s.onChunk != nil {
			if err := s.onChunk(chunk); err != nil {
				s.ended <- err

				return err
			}
		}
	}
}

func (s *transportServer) chunksFor(id string) []*sm.AdHocResultChunk {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]*sm.AdHocResultChunk(nil), s.chunks[id]...)
}

func (s *transportServer) waitEnded(t *testing.T) error {
	t.Helper()

	select {
	case err := <-s.ended:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for a ReportAdHocResult stream to end")
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

// bigLogProber logs a single, very large line.
type bigLogProber struct{ size int }

func (bigLogProber) Name() string { return "biglog" }

func (p bigLogProber) Probe(_ context.Context, _ string, _ *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	_ = l.Log("msg", strings.Repeat("x", p.size))

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

// Many executions at once, each on its own stream: results must not
// get mixed up.
func TestTransportConcurrentExecutions(t *testing.T) {
	const n = 50

	requests := make([]*sm.AdHocRequest, 0, n)
	for i := range n {
		requests = append(requests, transportRequest(fmt.Sprintf("exec-%d", i)))
	}

	srv := newTransportServer(requests...)

	publishCh := startTransportHandler(t, srv, func(ctx context.Context, req *sm.AdHocRequest) (*runner, error) {
		return proberFactory(func() proberLike { return &testProber{zerolog.New(io.Discard)} })(ctx, req)
	})

	for range n {
		require.NoError(t, srv.waitEnded(t), "every stream must close cleanly")
		waitPublished(t, publishCh)
	}

	for _, req := range requests {
		id := req.AdHocCheck.Id
		chunks := srv.chunksFor(id)

		require.GreaterOrEqual(t, len(chunks), 2, id)

		for _, c := range chunks {
			require.Equal(t, id, c.ExecutionId, "chunks from different executions got mixed up")
			require.Equal(t, int64(7), c.ProbeId)
		}

		final := finalOf(t, chunks)
		require.Equal(t, sm.AdHocFinal_OK, final.Status, id)
	}
}

// The default gRPC receive limit is 4MiB. A log line over our chunk
// limit is replaced by a marker, so the stream stays healthy and the API
// still gets the final result.
func TestTransportOversizedLogLine(t *testing.T) {
	srv := newTransportServer(transportRequest("big"))

	publishCh := startTransportHandler(t, srv,
		proberFactory(func() proberLike { return bigLogProber{size: 5 << 20} }))

	waitPublished(t, publishCh)

	require.NoError(t, srv.waitEnded(t), "the stream must close cleanly")

	chunks := srv.chunksFor("big")
	require.Equal(t, sm.AdHocFinal_OK, finalOf(t, chunks).Status)

	require.Len(t, chunks, 2, "one log chunk and the final one")

	lines := chunks[0].GetLogLines().Lines
	require.Len(t, lines, 1)
	require.Contains(t, string(lines[0]), "truncated")
	require.Less(t, len(lines[0]), maxResultChunkBytes)
}

// Many lines that don't fit in one chunk are split, and none are lost.
func TestTransportLogsAreSplitIntoChunks(t *testing.T) {
	const (
		lines = 100
		size  = 100 << 10 // 10MiB in total.
	)

	srv := newTransportServer(transportRequest("split"))

	var elapsed atomic.Int64

	publishCh := startTransportHandler(t, srv,
		proberFactory(func() proberLike { return manyLinesProber{n: lines, size: size, elapsed: &elapsed} }))

	waitPublished(t, publishCh)

	require.NoError(t, srv.waitEnded(t))

	chunks := srv.chunksFor("split")
	require.Greater(t, len(chunks), 2, "10MiB of logs cannot fit in a single chunk")

	var got int

	for _, c := range chunks[:len(chunks)-1] {
		var bytes int

		for _, l := range c.GetLogLines().Lines {
			bytes += len(l)
			got++
		}

		require.LessOrEqual(t, bytes, maxResultChunkBytes)
	}

	require.Equal(t, lines, got, "no log line may be lost")
	require.Equal(t, sm.AdHocFinal_OK, finalOf(t, chunks).Status)
}

// The API aborts the result stream. The check result in Loki is not
// affected.
func TestTransportStreamAborted(t *testing.T) {
	srv := newTransportServer(transportRequest("abort"))
	srv.onChunk = func(*sm.AdHocResultChunk) error {
		return status.Error(codes.Internal, "abort")
	}

	gate := make(chan struct{})
	close(gate)

	publishCh := startTransportHandler(t, srv,
		proberFactory(func() proberLike { return gatedProber{gate: gate} }))

	line := waitPublished(t, publishCh).Streams()[0].Entries[0].Line
	require.Contains(t, line, "first")
	require.Contains(t, line, "second")

	require.Error(t, srv.waitEnded(t), "the server aborted the stream")
}

// The connection that delivers requests drops and is re-established
// while a check is running. The result stream is independent: the
// result must still arrive.
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

	require.NoError(t, srv.waitEnded(t))
	waitPublished(t, publishCh)

	chunks := srv.chunksFor("reconnect")
	require.Len(t, chunks, 2, "one log chunk and the final one")
	require.Len(t, chunks[0].GetLogLines().Lines, 2, "first and second")
	require.Equal(t, sm.AdHocFinal_OK, finalOf(t, chunks).Status)
}

// manyLinesProber logs n lines and records how long Probe took.
type manyLinesProber struct {
	n       int
	size    int
	elapsed *atomic.Int64
}

func (manyLinesProber) Name() string { return "manylines" }

func (p manyLinesProber) Probe(_ context.Context, _ string, _ *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	start := time.Now()

	for i := range p.n {
		_ = l.Log("msg", fmt.Sprintf("line %d %s", i, strings.Repeat("x", p.size)))
	}

	p.elapsed.Store(int64(time.Since(start)))

	return true, 1
}

// A slow API must not slow the probe down. Results are only reported
// once the probe is done, so the probe never waits on the network.
func TestTransportSlowServerDoesNotDelayProbe(t *testing.T) {
	const (
		lines = 100
		size  = 16 << 10 // 1.6MiB in total, well over the 64KiB flow control window.
		delay = 20 * time.Millisecond
	)

	srv := newTransportServer(transportRequest("slow"))
	srv.onChunk = func(*sm.AdHocResultChunk) error {
		time.Sleep(delay)

		return nil
	}

	var elapsed atomic.Int64

	publishCh := startTransportHandler(t, srv,
		proberFactory(func() proberLike { return manyLinesProber{n: lines, size: size, elapsed: &elapsed} }))

	waitPublished(t, publishCh)

	got := time.Duration(elapsed.Load())
	t.Logf("probe logged %d lines of %d bytes in %v with a %v per-chunk server delay", lines, size, got, delay)

	require.Less(t, got, lines*delay/5, "the probe was held up by the slow server")
}

// manyMetricsProber registers n gauges, each with a large help string.
type manyMetricsProber struct{ n, size int }

func (manyMetricsProber) Name() string { return "manymetrics" }

func (p manyMetricsProber) Probe(_ context.Context, _ string, registry *prometheus.Registry, _ logger.Logger, _ string) (bool, float64) {
	for i := range p.n {
		registry.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{
			Name: fmt.Sprintf("test_metric_%03d", i),
			Help: strings.Repeat("h", p.size),
		}))
	}

	return true, 1
}

// Metrics that do not fit in a chunk are clipped, the caller is told,
// and what is left is still valid JSON.
func TestTransportMetricsTruncated(t *testing.T) {
	srv := newTransportServer(transportRequest("metrics"))

	publishCh := startTransportHandler(t, srv,
		proberFactory(func() proberLike { return manyMetricsProber{n: 100, size: 100 << 10} })) // ~10MiB.

	waitPublished(t, publishCh)

	require.NoError(t, srv.waitEnded(t), "the stream must close cleanly")

	final := finalOf(t, srv.chunksFor("metrics"))
	require.Equal(t, sm.AdHocFinal_OK, final.Status, "clipping metrics does not fail the check")

	require.LessOrEqual(t, len(final.Metrics), maxResultChunkBytes)
	require.True(t, json.Valid(final.Metrics), "clipped metrics must still be valid JSON")
	require.Contains(t, string(final.Metrics), "probe_success", "the leading metric families are kept")

	require.Len(t, final.Warnings, 1)
	require.Contains(t, final.Warnings[0], "truncated")
}

// Metrics that fit are sent as they are, without a warning.
func TestTransportMetricsNotTruncatedWhenSmall(t *testing.T) {
	srv := newTransportServer(transportRequest("small"))

	publishCh := startTransportHandler(t, srv,
		proberFactory(func() proberLike { return manyMetricsProber{n: 3, size: 10} }))

	waitPublished(t, publishCh)

	require.NoError(t, srv.waitEnded(t))

	final := finalOf(t, srv.chunksFor("small"))
	require.Empty(t, final.Warnings)
	require.Contains(t, string(final.Metrics), "test_metric_002")
}

// Clipping data is not silent for the operator: it is logged with the
// execution id, so that it can be found when a user reports missing data.
func TestTransportTruncationIsLogged(t *testing.T) {
	for name, tc := range map[string]struct {
		prober func() proberLike
		want   string
	}{
		"metrics":  {func() proberLike { return manyMetricsProber{n: 100, size: 100 << 10} }, "metrics truncated"},
		"log line": {func() proberLike { return bigLogProber{size: 5 << 20} }, "log line truncated"},
	} {
		t.Run(name, func(t *testing.T) {
			logs := &testhelper.LogBuffer{}
			srv := newTransportServer(transportRequest("logged"))

			publishCh := startTransportHandler(t, srv,
				func(_ context.Context, req *sm.AdHocRequest) (*runner, error) {
					return &runner{
						logger:  zerolog.New(logs),
						prober:  tc.prober(),
						id:      req.AdHocCheck.Id,
						target:  req.AdHocCheck.Target,
						probe:   "transportProbe",
						timeout: 10 * time.Second,
					}, nil
				})

			waitPublished(t, publishCh)

			require.NoError(t, srv.waitEnded(t))

			out := logs.String()
			require.Contains(t, out, tc.want)
			require.Contains(t, out, `"level":"warn"`)
			require.Contains(t, out, `"id":"logged"`)
		})
	}
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
