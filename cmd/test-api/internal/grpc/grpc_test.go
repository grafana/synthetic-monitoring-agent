package grpc

import (
	"context"
	"testing"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestNewServer(t *testing.T) {
	testCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	if deadline, ok := t.Deadline(); ok {
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		t.Cleanup(cancel)
		testCtx = ctx
	}

	s, err := NewServer(
		testCtx,
		&Opts{
			Logger:            zerolog.New(zerolog.NewTestWriter(t)),
			ListenAddr:        "0.0.0.0:",
			ChecksServer:      &testChecksServer{},
			TenantsServer:     &testTenantsServer{},
			AdHocChecksServer: &testAdHocChecksServer{},
			Db:                &testDb{},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, s)

	res := make(chan error)
	go func() {
		res <- s.Run(testCtx)
	}()

	time.Sleep(100 * time.Millisecond) // give the server some time to start
	s.Stop()

	select {
	case <-testCtx.Done():
		require.False(t, true, "deadline exceeded")
	case err := <-res:
		if err != nil {
			require.ErrorIs(t, err, grpc.ErrServerStopped)
		}
	}
}

type testChecksServer struct{}

func (s *testChecksServer) RegisterProbe(context.Context, *sm.ProbeInfo) (*sm.RegisterProbeResult, error) {
	return nil, nil
}

func (s *testChecksServer) GetChanges(*sm.ProbeState, sm.Checks_GetChangesServer) error {
	return nil
}

func (s *testChecksServer) Ping(context.Context, *sm.PingRequest) (*sm.PongResponse, error) {
	return nil, nil
}

type testTenantsServer struct{}

func (s *testTenantsServer) GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error) {
	return nil, nil
}

type testAdHocChecksServer struct{}

func (s *testAdHocChecksServer) RegisterProbe(context.Context, *sm.ProbeInfo) (*sm.RegisterProbeResult, error) {
	return nil, nil
}

func (s *testAdHocChecksServer) GetAdHocChecks(*sm.Void, sm.AdHocChecks_GetAdHocChecksServer) error {
	return nil
}

type testDb struct{}

func (db *testDb) FindProbeIDByToken(ctx context.Context, token []byte) (int64, error) {
	return 0, nil
}
