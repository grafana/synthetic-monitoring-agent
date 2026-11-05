package checks

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/scraper"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type testStateCache struct{}

func (*testStateCache) Store(context.Context, model.Check, time.Time, scraper.TimeSeries) {}

func (*testStateCache) Load(context.Context, model.Check) (scraper.StateRecord, scraper.TimeSeries, bool) {
	return scraper.StateRecord{}, nil, false
}

func TestNewUpdaterStateCache(t *testing.T) {
	base := func(t *testing.T) UpdaterOptions {
		t.Helper()

		return UpdaterOptions{
			Conn:           new(grpc.ClientConn),
			PromRegisterer: prometheus.NewPedanticRegistry(),
			Publisher:      channelPublisher(make(chan pusher.Payload)),
			TenantCh:       make(chan<- sm.Tenant),
			Logger:         testhelper.Logger(t),
			Features:       feature.NewCollection(),
		}
	}

	t.Run("unset hands scrapers a noop writer", func(t *testing.T) {
		u, err := NewUpdater(base(t))
		require.NoError(t, err)
		require.IsType(t, scraper.NoopStateCache{}, u.stateCache)
	})

	t.Run("set hands scrapers the caller's writer", func(t *testing.T) {
		writer := &testStateCache{}

		opts := base(t)
		opts.StateCache = writer

		u, err := NewUpdater(opts)
		require.NoError(t, err)
		require.Same(t, writer, u.stateCache)
	})
}
