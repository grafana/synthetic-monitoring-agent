package checks

import (
	"context"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/scraper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
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
				Publisher:      channelPublisher(make(chan pusher.Payload)),
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
			require.Equal(t, tc.opts.Publisher, u.publisher)
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
	publishCh := make(chan pusher.Payload, 100)

	u, err := NewUpdater(
		UpdaterOptions{
			Conn:           new(grpc.ClientConn),
			PromRegisterer: prometheus.NewPedanticRegistry(),
			Publisher:      channelPublisher(publishCh),
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

	var check model.Check
	err = check.FromSM(sm.Check{
		Id:        5000,
		TenantId:  1,
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
	})
	require.NoError(t, err)

	scraperExists := func() bool {
		u.scrapersMutex.Lock()
		defer u.scrapersMutex.Unlock()
		_, found := u.scrapers[check.GlobalID()]
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

type testProbeFactory struct {
}

func (f testProbeFactory) New(ctx context.Context, logger zerolog.Logger, check model.Check) (prober.Prober, string, error) {
	return testProber{}, check.Target, nil
}

func testScraperFactory(ctx context.Context, check model.Check, publisher pusher.Publisher, _ sm.Probe, logger zerolog.Logger, scrapeCounter scraper.Incrementer, scrapeErrorCounter scraper.IncrementerVec, k6Runner k6runner.Runner) (*scraper.Scraper, error) {
	return scraper.NewWithOpts(
		ctx,
		check,
		scraper.ScraperOpts{
			ErrorCounter:  scrapeErrorCounter,
			Logger:        logger,
			ProbeFactory:  testProbeFactory{},
			Publisher:     publisher,
			ScrapeCounter: scrapeCounter,
		},
	)
}

type channelPublisher chan pusher.Payload

func (c channelPublisher) Publish(payload pusher.Payload) {
	c <- payload
}
