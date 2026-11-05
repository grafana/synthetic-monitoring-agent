package checks

import (
	"strings"
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
	const metricPrefix = "sm_agent_scraper_cache_"

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

	cacheMetricNames := func(t *testing.T, reg prometheus.Gatherer) []string {
		t.Helper()

		mfs, err := reg.Gather()
		require.NoError(t, err)

		var names []string

		for _, mf := range mfs {
			if strings.HasPrefix(mf.GetName(), metricPrefix) {
				names = append(names, mf.GetName())
			}
		}

		return names
	}

	t.Run("nil cache disables the state writer", func(t *testing.T) {
		opts := base(t)

		u, err := NewUpdater(opts)
		require.NoError(t, err)
		require.NotNil(t, u.stateWriter)
		require.False(t, u.stateWriter.Enabled())
		require.Empty(t, cacheMetricNames(t, opts.PromRegisterer.(prometheus.Gatherer)))
	})

	t.Run("a cache enables the state writer and its metrics", func(t *testing.T) {
		opts := base(t)
		opts.ScraperCache = cache.NewNoop(opts.Logger)

		u, err := NewUpdater(opts)
		require.NoError(t, err)
		require.True(t, u.stateWriter.Enabled())
		require.ElementsMatch(t,
			[]string{metricPrefix + "record_bytes", metricPrefix + "writes_total"},
			cacheMetricNames(t, opts.PromRegisterer.(prometheus.Gatherer)),
		)
	})
}
