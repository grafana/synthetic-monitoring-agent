package checks

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/grafana/synthetic-monitoring-agent/internal/cache"
	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

func TestNewUpdaterScraperCache(t *testing.T) {
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

	t.Run("defaults to a noop cache", func(t *testing.T) {
		u, err := NewUpdater(base(t))
		require.NoError(t, err)
		require.IsType(t, &cache.Noop{}, u.scraperCache)
	})

	t.Run("keeps the provided cache", func(t *testing.T) {
		opts := base(t)
		opts.ScraperCache = cache.NewNoop(opts.Logger)

		u, err := NewUpdater(opts)
		require.NoError(t, err)
		require.Same(t, opts.ScraperCache, u.scraperCache)
	})
}
