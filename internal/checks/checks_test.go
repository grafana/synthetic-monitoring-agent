package checks

import (
	"context"
	"fmt"
	"math"
	"net"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/scraper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/jpillora/backoff"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNewUpdater(t *testing.T) {
	testFeatureCollection := feature.NewCollection()
	require.NotNil(t, testFeatureCollection)
	require.NoError(t, testFeatureCollection.Set("foo"))
	require.NoError(t, testFeatureCollection.Set("bar"))

	testcases := map[string]struct {
		opts UpdaterOptions
	}{
		"trivial": {
			opts: UpdaterOptions{
				Conn:           new(grpc.ClientConn),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				PublishCh:      make(chan<- pusher.Payload),
				TenantCh:       make(chan<- sm.Tenant),
				Logger:         zerolog.Nop(),
				Features:       testFeatureCollection,
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			u, err := NewUpdater(tc.opts)
			require.NoError(t, err)
			require.NotNil(t, u)
			require.Equal(t, tc.opts.PublishCh, u.publishCh)
			require.Equal(t, tc.opts.TenantCh, u.tenantCh)
			require.Equal(t, tc.opts.Features, u.features)
			require.Equal(t, tc.opts.Logger, u.logger)
			require.Equal(t, tc.opts.Conn, u.api.conn)
			require.NotNil(t, u.scrapers)
			require.NotNil(t, u.metrics.changesCounter)
			require.NotNil(t, u.metrics.changeErrorsCounter)
			require.NotNil(t, u.metrics.runningScrapers)
			require.NotNil(t, u.metrics.scrapesCounter)
			require.NotNil(t, u.metrics.scrapeErrorCounter)
			require.NotNil(t, u.metrics.probeInfo)
		})
	}
}

func TestPing(t *testing.T) {
	// Tests that the Updater is able to timeout when the server
	// goes unresponsive without terminating the grpc connection.
	for _, test := range []*crashServer{
		{},
		{allowRegister: true},
		{allowRegister: true, allowedPings: 1},
		{allowRegister: true, allowedPings: 2},
	} {
		t.Run(test.String(), func(t *testing.T) {
			server := sm.ChecksServer(test)
			t.Parallel()
			testNetworkFailure(t, server)
		})
	}
}

func testNetworkFailure(t *testing.T, s sm.ChecksServer) {
	// Bind a listener to any available port
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	// Create a grpc server
	server := grpc.NewServer()
	defer server.Stop()

	// Register the crashServer as checks server.
	sm.RegisterChecksServer(server, s)

	// Serve requests.
	go func() {
		if err := server.Serve(listener); err != nil {
			t.Logf("server: %v", err)
		}
	}()
	t.Logf("running fake server at %s", listener.Addr())

	// Create a grpc client and updater.
	conn, err := grpc.Dial(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	u, err := NewUpdater(UpdaterOptions{
		Backoff: &backoff.Backoff{
			Min:    2 * time.Second,
			Max:    30 * time.Second,
			Factor: math.Pow(30./2., 1./8.), // reach the target in ~ 8 steps
			Jitter: true,
		},
		Conn:           conn,
		Features:       feature.NewCollection(),
		IsConnected:    func(b bool) {},
		Logger:         zerolog.New(zerolog.NewTestWriter(t)),
		PromRegisterer: prometheus.NewPedanticRegistry(),
		PublishCh:      make(chan<- pusher.Payload),
		TenantCh:       make(chan<- sm.Tenant),
	})
	require.NoError(t, err)
	require.NotNil(t, u)

	// Reduce the ping settings so the test doesn't take too long.
	u.pingTimeout = time.Second * 3
	u.pingInterval = time.Second * 5

	// How long to wait before considering the Updater hanged.
	const failureTimeout = time.Minute

	// Run the updater and a sleep in parallel to stop the test
	// if a hang is detected
	ctx, cancel := context.WithCancel(context.Background())
	sleepErrC := make(chan error, 1)
	go func() {
		err := sleepCtx(ctx, failureTimeout)
		if err == nil {
			// sleep completed without ctx being signaled.
			cancel()
		}
		sleepErrC <- err
	}()
	go func() {
		err := u.Run(ctx)
		t.Log(err)
		cancel()
	}()

	// If the err read from sleepErrC is not nil, Updater.Run() finished
	// before the timeout elapsed, so it didn't hang.
	err = <-sleepErrC
	require.Equal(t, context.Canceled, err)
}

func TestInstallSignalHandler(t *testing.T) {
	testcases := map[string]func(t *testing.T){
		"signal": func(t *testing.T) {
			// verify that the signal context is done after
			// receiving the signal, and that the signal is
			// correctly reported as having fired.

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			sigCtx, signalFired := installSignalHandler(ctx)
			require.NotNil(t, sigCtx)
			require.NotNil(t, signalFired)
			require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGUSR1))

			select {
			case <-ctx.Done():
				t.Fatal("context timeout expired")
			case <-sigCtx.Done():
				require.Equal(t, int32(1), atomic.LoadInt32(signalFired))
			}
		},

		"no signal": func(t *testing.T) {
			// verify that the signal context is done after
			// the parrent context is done, and that the
			// signal is correctly reported as not having
			// fired.

			ctx, cancel := context.WithCancel(context.Background())
			sigCtx, signalFired := installSignalHandler(ctx)
			require.NotNil(t, sigCtx)
			require.NotNil(t, signalFired)

			cancel()

			timeout := 100 * time.Millisecond
			timer := time.NewTimer(timeout)
			defer timer.Stop()

			select {
			case <-timer.C:
				t.Fatalf("signal context not cancelled after %s", timeout)
			case <-sigCtx.Done():
				require.Equal(t, int32(0), atomic.LoadInt32(signalFired))
			}
		},
	}

	for name, f := range testcases {
		t.Run(name, f)
	}
}

func TestSleepCtx(t *testing.T) {
	var (
		veryShort = 1 * time.Microsecond
		long      = 10 * time.Second
	)

	// make sure errors are reported correctly

	ctx := context.Background()
	err := sleepCtx(ctx, veryShort)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = sleepCtx(ctx, long)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)

	ctx, cancel = context.WithTimeout(context.Background(), veryShort)
	err = sleepCtx(ctx, long)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	cancel()

	ctx, cancel = context.WithTimeout(context.Background(), long)
	cancel()
	err = sleepCtx(ctx, long)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// TestHandleCheckOp is testing internal functions that run as part of
// updater.Run. Since these functions operate on scraper instances, a
// test scraper is used, which in turn creates a test probe. The goal of
// this is to decouple the testing of these functions from the testing
// of the prober themselves.
func TestHandleCheckOp(t *testing.T) {
	publishCh := make(chan<- pusher.Payload, 100)

	u, err := NewUpdater(
		UpdaterOptions{
			Conn:           new(grpc.ClientConn),
			PromRegisterer: prometheus.NewPedanticRegistry(),
			PublishCh:      publishCh,
			TenantCh:       make(chan<- sm.Tenant),
			Logger:         zerolog.Nop(),
			ScraperFactory: testScraperFactory,
		},
	)

	require.NotNil(t, u)
	require.NoError(t, err)

	u.probe = &sm.Probe{
		Id:   100,
		Name: "test-probe",
	}

	deadline, ok := t.Deadline()
	if !ok {
		deadline = time.Now().Add(2 * time.Second)
	}

	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	check := sm.Check{
		Id:        5000,
		Frequency: 1000,
		Timeout:   1000,
		Target:    "127.0.0.1",
		Job:       "", // not setting value to make check invalid
		Probes:    []int64{1},
		Settings: sm.CheckSettings{
			Ping: &sm.PingSettings{},
		},
		Created:  0,
		Modified: 0,
	}

	scraperExists := func() bool {
		u.scrapersMutex.Lock()
		defer u.scrapersMutex.Unlock()
		_, found := u.scrapers[check.Id]
		return found
	}

	// this should fail, check is invalid
	err = u.handleCheckAdd(ctx, check)
	require.Error(t, err)
	// This doesn't work because the counter hasn't been set
	// (because of the error):
	// require.Equal(t, 0.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.False(t, scraperExists())

	// fix check
	check.Job = "test-job"
	check.Modified++

	err = u.handleCheckAdd(ctx, check)
	require.NoError(t, err)
	require.Equal(t, 1.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.True(t, scraperExists())

	check.Modified++

	// try to add again, this should fail, even if modified changed
	err = u.handleCheckAdd(ctx, check)
	require.Error(t, err)
	require.Equal(t, 1.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.True(t, scraperExists())

	check.Modified++

	// update the existing check
	err = u.handleCheckUpdate(ctx, check)
	require.NoError(t, err)
	require.Equal(t, 1.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.True(t, scraperExists())

	err = u.handleCheckDelete(ctx, check)
	require.NoError(t, err)
	require.Equal(t, 0.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.False(t, scraperExists())

	// try to delete again
	err = u.handleCheckDelete(ctx, check)
	require.Error(t, err)
	require.Equal(t, 0.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.False(t, scraperExists())

	// updating a non-existing check becomes an add
	err = u.handleCheckUpdate(ctx, check)
	require.NoError(t, err)
	require.Equal(t, 1.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.True(t, scraperExists())

	// clean up
	err = u.handleCheckDelete(ctx, check)
	require.NoError(t, err)
	require.Equal(t, 0.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.False(t, scraperExists())
}

type testProber struct {
}

func (testProber) Name() string {
	return "test-prober"
}

func (testProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	return false
}

func testProbeFactory(ctx context.Context, logger zerolog.Logger, check sm.Check) (prober.Prober, string, error) {
	return testProber{}, check.Target, nil
}

func testScraperFactory(ctx context.Context, check sm.Check, payloadCh chan<- pusher.Payload, _ sm.Probe, logger zerolog.Logger, scrapeCounter prometheus.Counter, scrapeErrorCounter *prometheus.CounterVec) (*scraper.Scraper, error) {
	return scraper.NewWithOpts(
		ctx,
		check,
		scraper.ScraperOpts{
			ErrorCounter:  scrapeErrorCounter,
			Logger:        logger,
			ProbeFactory:  testProbeFactory,
			PublishCh:     payloadCh,
			ScrapeCounter: scrapeCounter,
		},
	)
}

// crashServer is used to simulate an API server that stops responding
// to grpc requests. This is equivalent to a server that suddenly disappears
// from the network without properly terminating the grpc connection.
type crashServer struct {
	// if allowRegister is true, it will process a RegisterProbe request.
	// Otherwise it will wait forever.
	allowRegister bool
	// allowedPings defines how many pings are answered before ignoring them.
	allowedPings int
}

func (s *crashServer) RegisterProbe(ctx context.Context, _ *sm.ProbeInfo) (*sm.RegisterProbeResult, error) {
	if s.allowRegister {
		return &sm.RegisterProbeResult{
			Probe: sm.Probe{
				Id:       1234,
				TenantId: 1234,
			},
			Status: sm.Status{
				Code: sm.StatusCode_OK,
			},
		}, nil
	}
	return nil, s.wait(ctx)
}
func (s *crashServer) GetChanges(_ *sm.Void, srv sm.Checks_GetChangesServer) error {
	// GetChanges always blocks, as a changes stream that never returns any changes.
	return s.wait(srv.Context())
}

func (s *crashServer) Ping(ctx context.Context, req *sm.PingRequest) (*sm.PongResponse, error) {
	if s.allowedPings > 0 {
		s.allowedPings--
		return &sm.PongResponse{
			Sequence: req.Sequence,
		}, nil
	}
	return nil, s.wait(ctx)
}

func (s *crashServer) String() string {
	return fmt.Sprintf("allowed requests register=%v pings=%d",
		s.allowRegister, s.allowedPings)
}

func (*crashServer) wait(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

var _ sm.ChecksServer = (*crashServer)(nil)
