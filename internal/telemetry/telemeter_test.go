package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	prom "github.com/prometheus/client_golang/prometheus"
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
		timeSpan   = 1 * time.Hour
		testClient = &testTelemetryClient{
			wg: nil, // this is handled gracefully.
			rr: testPushResp{
				tr: &sm.PushTelemetryResponse{
					Status: &sm.Status{Code: sm.StatusCode_OK},
				},
			},
		}
		registerer = prom.NewPedanticRegistry()
	)

	// Create a cancellable context to control goroutine lifecycle
	ctx, cancel := context.WithCancel(t.Context())

	// Ensure all goroutines are stopped before test ends
	t.Cleanup(func() {
		cancel()
		// Give goroutines time to exit after context cancellation
		time.Sleep(200 * time.Millisecond)
	})

	tele := NewTelemeter(ctx, instance, timeSpan, testClient, testhelper.Logger(t), registerer)

	{ // should init with no region pushers
		verifyTelemeter(t, tele, 0)
	}

	{ // should create a new region pusher
		execution := getTestDataset(0).executions[0]
		tele.AddExecution(execution)
		verifyTelemeter(t, tele, 1)
		verifyRegionPusher(t, tele, execution.RegionID, execution)
	}

	{ // should add telemetry to current region pusher
		executions := getTestDataset(0).executions
		tele.AddExecution(executions[1])
		tele.AddExecution(executions[2])
		verifyTelemeter(t, tele, 1)
		verifyRegionPusher(t, tele, executions[0].RegionID, executions[:2]...)
	}

	{ // should add another region pusher
		executions := getTestDataset(0).executions
		executions[2].RegionID = 1
		tele.AddExecution(executions[2])
		executions[3].RegionID = 1
		tele.AddExecution(executions[3])
		verifyTelemeter(t, tele, 2)
		verifyRegionPusher(t, tele, executions[3].RegionID, executions[2:4]...)
	}

	{ // initial region pusher data should be intact
		executions := getTestDataset(0).executions
		verifyRegionPusher(t, tele, executions[0].RegionID, executions[:2]...)
	}
}
