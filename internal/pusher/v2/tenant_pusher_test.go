package v2

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

func TestTenantPusher(t *testing.T) {
	// This is an extremely basic test that verifies that a tenant pusher
	// can be constructed.
	tenantProvider := testTenantProvider{
		1: &sm.Tenant{
			Id:            1,
			OrgId:         1,
			MetricsRemote: &sm.RemoteInfo{},
			EventsRemote:  &sm.RemoteInfo{},
			Status:        sm.TenantStatus_ACTIVE,
		},
	}

	registry := prometheus.NewPedanticRegistry()
	metrics := pusher.NewMetrics(registry)

	p := newTenantPusher(1, tenantProvider, pusherOptions{
		metrics: metrics,
	})
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(50*time.Millisecond))
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		p.run(ctx)
		wg.Done()
	}()
	wg.Wait()
}

func TestTenantPusherRunPushers(t *testing.T) {
	// This test will make sure that runPushers handles error conditions correctly.
	tenantProvider := testTenantProvider{
		1: &sm.Tenant{
			Id:            1,
			OrgId:         1,
			MetricsRemote: &sm.RemoteInfo{},
			EventsRemote:  &sm.RemoteInfo{},
			Status:        sm.TenantStatus_ACTIVE,
		},
	}

	registry := prometheus.NewPedanticRegistry()
	metrics := pusher.NewMetrics(registry).WithTenant(2, 1)

	// use tenant ID 2 to force an error
	p := newTenantPusher(2, tenantProvider, pusherOptions{
		metrics: metrics,
	})

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(50*time.Millisecond))
	defer cancel()

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		// Make sure the deadline is handled. If the runPushers method
		// below returns before the deadline, the group context will be
		// cancelled and the line below will return. If the deadline is
		// met, the line below will return instead (if runPushers
		// doesn't handle it correctly).
		<-gCtx.Done()
		return gCtx.Err()
	})

	g.Go(func() error { return p.runPushers(gCtx) })

	err := g.Wait()

	require.Error(t, err)
	require.ErrorIs(t, err, errTestNoTenant)
}

func makeRecords(blocks [][]byte) []queueEntry {
	out := make([]queueEntry, len(blocks))
	for idx, b := range blocks {
		data := make([]byte, len(b))
		copy(data, b)
		out[idx].data = &data
	}
	return out
}

type testTenantProvider map[int64]*sm.Tenant

var errTestNoTenant = errors.New("tenant not found")

func (t testTenantProvider) GetTenant(ctx context.Context, info *sm.TenantInfo) (*sm.Tenant, error) {
	tenant, found := t[info.Id]
	if !found {
		return nil, errTestNoTenant
	}
	return tenant, nil
}

type testServer struct {
	mu           sync.Mutex
	receivedBody []byte
	responses    []http.HandlerFunc
	server       *httptest.Server
}

func (s *testServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.responses) == 0 {
		panic(len(s.responses))
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	content, err := snappy.Decode(nil, body)
	if err != nil {
		panic(err)
	}
	s.receivedBody = content
	defer r.Body.Close()
	act := s.responses[0]
	s.responses = s.responses[1:]
	act(w, r)
}

func (s *testServer) start() {
	s.server = httptest.NewServer(s)
}

func (s *testServer) stop() {
	s.server.Close()
}

func (s *testServer) done() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.responses) == 0
}
