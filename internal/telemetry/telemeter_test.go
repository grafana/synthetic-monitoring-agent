package telemetry

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestTelemeterAddExecution(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		verifyTelemeter := func(t *testing.T, tele *Telemeter, nRegionPushers int) {
			t.Helper()
			require.Equal(t, len(tele.pushers), nRegionPushers)
		}

		verifyRegionPusher := func(t *testing.T, tele *Telemeter, regionID int32, ee ...Execution) {
			t.Helper()
			p, ok := tele.pushers[regionID]
			require.True(t, ok)

			// sum expected executions data, indexed by tenant -> checkClass -> calKey
			regionTele := make(map[int64]map[sm.CheckClass]map[string]*sm.CheckClassTelemetry)
			for _, e := range ee {
				tenantTele, ok := regionTele[e.LocalTenantID]
				if !ok {
					tenantTele = make(map[sm.CheckClass]map[string]*sm.CheckClassTelemetry)
					regionTele[e.LocalTenantID] = tenantTele
				}
				clTele, ok := tenantTele[e.CheckClass]
				if !ok {
					clTele = make(map[string]*sm.CheckClassTelemetry)
					tenantTele[e.CheckClass] = clTele
				}
				calKey := serializeCALs(e.CostAttributionLabels)
				calTele, ok := clTele[calKey]
				if !ok {
					calTele = &sm.CheckClassTelemetry{
						CheckClass:            e.CheckClass,
						CostAttributionLabels: deserializeCals(calKey),
					}
					clTele[calKey] = calTele
				}
				calTele.Executions++
				calTele.Duration += float32(e.Duration.Seconds())
			}

			// verify
			for tenantID, expTTele := range regionTele {
				gotTTele, ok := p.telemetry[tenantID]
				require.True(t, ok, "telemetry not found for tenant %d", tenantID)
				for checkClass, expCLTele := range expTTele {
					gotCLTele, ok := gotTTele[checkClass]
					require.True(t, ok, "telemetry not found for check class %v", checkClass)
					for calKey, expCalTele := range expCLTele {
						gotCalTele, ok := gotCLTele[calKey]
						require.True(t, ok, "telemetry not found for cal key %s", calKey)
						require.Equal(t, expCalTele.Executions, gotCalTele.Executions, "executions mismatch for cal key %s", calKey)
						require.Equal(t, expCalTele.Duration, gotCalTele.Duration, "duration mismatch for cal key %s", calKey)
						require.Equal(t, expCalTele.CostAttributionLabels, gotCalTele.CostAttributionLabels, "cals mispath for cal key %s", calKey)
					}
				}
			}
		}

		var (
			timeSpan   = 1 * time.Hour
			testClient = &testTelemetryClient{
				rr: testPushResp{
					tr: &sm.PushTelemetryResponse{
						Status: &sm.Status{Code: sm.StatusCode_OK},
					},
				},
			}
			registerer = prom.NewPedanticRegistry()
		)

		// Create context within synctest bubble.
		//
		// Do not use t.Context() here because that's derived from the main context created outside the bubble.
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

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
	})
}
