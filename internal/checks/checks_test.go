package checks

import (
	"testing"

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
