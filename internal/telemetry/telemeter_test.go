package telemetry

import (
	"context"
	"testing"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestTelemeterAddExecution(t *testing.T) {
	verifyTelemeter := func(t *testing.T, tele *Telemeter, nRegionPushers int) {
		t.Helper()
		require.Equal(t, len(tele.pushers), nRegionPushers)
	}

	verifyRegionPusher := func(t *testing.T, tele *Telemeter, regionID int32, ee ...Execution) {
		t.Helper()
		p, ok := tele.pushers[regionID]
		require.True(t, ok)

		// sum expected executions data
		regionTele := make(map[int64]map[sm.CheckClass]*sm.CheckClassTelemetry)
		for _, e := range ee {
			tenantTele, ok := regionTele[e.LocalTenantID]
			if !ok {
				tenantTele = make(map[sm.CheckClass]*sm.CheckClassTelemetry)
				regionTele[e.LocalTenantID] = tenantTele
			}
			if _, ok := tenantTele[e.CheckClass]; !ok {
				tenantTele[e.CheckClass] = &sm.CheckClassTelemetry{CheckClass: e.CheckClass}
			}
			tenantTele[e.CheckClass].Executions++
			tenantTele[e.CheckClass].Duration += float32(e.Duration.Seconds())
		}

		// verify
		for tenantID, expTTele := range regionTele {
			gotTTele, ok := p.telemetry[tenantID]
			require.True(t, ok, "telemetry not found for tenant")
			for _, expCCTele := range expTTele {
				gotCCTele, ok := gotTTele[expCCTele.CheckClass]
				require.True(t, ok, "telemetry not found for check class")
				require.Equal(t, expCCTele.Executions, gotCCTele.Executions)
				require.Equal(t, expCCTele.Duration, gotCCTele.Duration)
			}
		}
	}

	var (
		ctx        = context.Background()
		timeSpan   = 1 * time.Hour
		testClient = &testTelemetryClient{}
		registerer = prom.NewPedanticRegistry()
	)

	tele := NewTelemeter(ctx, instance, timeSpan, testClient, zerolog.Nop(), registerer)

	t.Run("should init with no region pushers", func(t *testing.T) {
		verifyTelemeter(t, tele, 0)
	})

	t.Run("should create a new region pusher", func(t *testing.T) {
		tele.AddExecution(ee[0])
		verifyTelemeter(t, tele, 1)
		verifyRegionPusher(t, tele, ee[0].RegionID, ee[0])
	})

	t.Run("should add telemetry to current region pusher", func(t *testing.T) {
		tele.AddExecution(ee[1])
		tele.AddExecution(ee[2])
		verifyTelemeter(t, tele, 1)
		verifyRegionPusher(t, tele, ee[0].RegionID, ee[:2]...)
	})

	t.Run("should add another region pusher", func(t *testing.T) {
		e := ee[2]
		e.RegionID = 1
		tele.AddExecution(e)
		e = ee[3]
		e.RegionID = 1
		tele.AddExecution(e)
		verifyTelemeter(t, tele, 2)
		verifyRegionPusher(t, tele, e.RegionID, ee[2:4]...)
	})

	t.Run("initial region pusher data should be intact", func(t *testing.T) {
		verifyRegionPusher(t, tele, ee[0].RegionID, ee[:2]...)
	})
}
