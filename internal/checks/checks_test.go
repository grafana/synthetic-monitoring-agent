package checks

import (
	"context"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
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
