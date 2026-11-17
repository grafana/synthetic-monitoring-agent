package adhoc

import (
	"context"
	"errors"
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
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// flakyTransport serves an API that can be taken down and brought back
// up, like a connection that drops while a long check is running.
type flakyTransport struct {
	impl sm.AdHocChecksServer

	down atomic.Bool

	mu  sync.Mutex
	lis *bufconn.Listener
	srv *grpc.Server
}

func newFlakyTransport(t *testing.T, impl sm.AdHocChecksServer) (*flakyTransport, *grpc.ClientConn) {
	t.Helper()

	f := &flakyTransport{impl: impl}
	f.serve()

	t.Cleanup(f.stop)

	cc, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			if f.down.Load() {
				return nil, errors.New("API is down")
			}

			f.mu.Lock()
			lis := f.lis
			f.mu.Unlock()

			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() { _ = cc.Close() })

	return f, cc
}

func (f *flakyTransport) serve() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.lis = bufconn.Listen(1 << 20)
	f.srv = grpc.NewServer()

	sm.RegisterAdHocChecksServer(f.srv, f.impl)

	lis, srv := f.lis, f.srv

	go func() { _ = srv.Serve(lis) }()
}

func (f *flakyTransport) stop() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.srv.Stop()
}

// dropAPI takes the API down, closing every open connection.
func (f *flakyTransport) dropAPI() {
	f.down.Store(true)
	f.stop()
}

// restoreAPI brings the API back.
func (f *flakyTransport) restoreAPI() {
	f.serve()
	f.down.Store(false)
}

// startedProber signals that it started, and waits for the gate.
type startedProber struct {
	started chan<- struct{}
	gate    <-chan struct{}
}

func (startedProber) Name() string { return "started" }

func (p startedProber) Probe(ctx context.Context, _ string, _ *prometheus.Registry, _ logger.Logger, _ string) (bool, float64) {
	close(p.started)

	select {
	case <-p.gate:
		return true, 1
	case <-ctx.Done():
		return false, 0
	}
}

// A long check outlives the connection it was started on. The API is
// down while the check finishes and comes back shortly afterwards, which
// is well within the time the probe waits for it: the result must not be
// lost.
func TestTransportAPIDownWhenCheckFinishes(t *testing.T) {
	srv := newTransportServer(transportRequest("long-check"))

	flaky, cc := newFlakyTransport(t, srv)

	features := feature.NewCollection()
	require.NoError(t, features.Set("adhoc"))

	started := make(chan struct{})
	gate := make(chan struct{})

	h, err := NewHandler(HandlerOpts{
		Conn:           cc,
		Logger:         zerolog.New(io.Discard),
		Backoff:        testBackoff{},
		Publisher:      channelPublisher(make(chan pusher.Payload, 8)),
		TenantCh:       make(chan sm.Tenant, 1),
		PromRegisterer: prometheus.NewPedanticRegistry(),
		Features:       features,
		runnerFactory: proberFactory(func() proberLike {
			return startedProber{started: started, gate: gate}
		}),
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
		<-done
	})

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the check did not start")
	}

	// The API goes away while the check is running, and the probe
	// notices.
	flaky.dropAPI()
	time.Sleep(500 * time.Millisecond)

	// The check finishes while the API is down.
	close(gate)

	// The API comes back, soon after.
	time.Sleep(500 * time.Millisecond)
	flaky.restoreAPI()

	result := srv.waitReported(t)
	require.Equal(t, "long-check", result.ExecutionId)
	require.Equal(t, sm.AdHocResult_OK, result.Status)
}
