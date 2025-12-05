package telemetry

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"

	prom "github.com/prometheus/client_golang/prometheus"
	prommodel "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

const (
	instance = "instance"
	regionID = 1
)

type testData struct {
	executions []Execution
	message    sm.RegionTelemetry
}

func getTestDataset(idx int) testData {
	data := []testData{
		{
			executions: []Execution{
				{
					LocalTenantID: 1,
					CheckClass:    sm.CheckClass_PROTOCOL,
					Duration:      59 * time.Second,
				},
				{
					LocalTenantID: 1,
					CheckClass:    sm.CheckClass_PROTOCOL,
					Duration:      60 * time.Second,
				},
				{
					LocalTenantID: 2,
					CheckClass:    sm.CheckClass_SCRIPTED,
					Duration:      61 * time.Second,
				},
				{
					LocalTenantID: 2,
					CheckClass:    sm.CheckClass_SCRIPTED,
					Duration:      30 * time.Second,
				},
				{
					LocalTenantID: 3,
					CheckClass:    sm.CheckClass_BROWSER,
					Duration:      61 * time.Second,
				},
				{
					LocalTenantID: 3,
					CheckClass:    sm.CheckClass_BROWSER,
					Duration:      30 * time.Second,
				},
				{
					LocalTenantID: 3,
					CheckClass:    sm.CheckClass_BROWSER,
					Duration:      30 * time.Second,
					CostAttributionLabels: []sm.CostAttributionLabel{
						{
							Name:  "env",
							Value: "prod",
						},
						{
							Name:  "team",
							Value: "__MISSING__",
						},
					},
				},
				{
					LocalTenantID: 3,
					CheckClass:    sm.CheckClass_BROWSER,
					Duration:      30 * time.Second,
					CostAttributionLabels: []sm.CostAttributionLabel{
						{
							Name:  "team",
							Value: "a",
						},
						{
							Name:  "env",
							Value: "__MISSING__",
						},
					},
				},
			},
			message: sm.RegionTelemetry{
				Instance: instance,
				RegionId: 1,
				Telemetry: []*sm.TenantTelemetry{
					{
						TenantId: 1,
						Telemetry: []*sm.CheckClassTelemetry{
							{
								CheckClass:            sm.CheckClass_PROTOCOL,
								Executions:            2,
								Duration:              119,
								SampledExecutions:     2,
								CostAttributionLabels: []sm.CostAttributionLabel{},
							},
						},
					},
					{
						TenantId: 2,
						Telemetry: []*sm.CheckClassTelemetry{
							{
								CheckClass:        sm.CheckClass_SCRIPTED,
								Executions:        2,
								Duration:          91,
								SampledExecutions: 3,

								CostAttributionLabels: []sm.CostAttributionLabel{},
							},
						},
					},
					{
						TenantId: 3,
						Telemetry: []*sm.CheckClassTelemetry{
							{
								CheckClass:            sm.CheckClass_BROWSER,
								Executions:            2,
								Duration:              91,
								SampledExecutions:     3,
								CostAttributionLabels: []sm.CostAttributionLabel{},
							},
							{
								CheckClass:        sm.CheckClass_BROWSER,
								Executions:        1,
								Duration:          30,
								SampledExecutions: 1,
								CostAttributionLabels: []sm.CostAttributionLabel{
									{
										Name:  "env",
										Value: "prod",
									},
									{
										Name:  "team",
										Value: "__MISSING__",
									},
								},
							},
							{
								CheckClass:        sm.CheckClass_BROWSER,
								Executions:        1,
								Duration:          30,
								SampledExecutions: 1,
								CostAttributionLabels: []sm.CostAttributionLabel{
									{
										Name:  "env",
										Value: "__MISSING__",
									},
									{
										Name:  "team",
										Value: "a",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			executions: []Execution{
				{
					LocalTenantID: 1,
					CheckClass:    sm.CheckClass_SCRIPTED,
					Duration:      30 * time.Second,
				},
				{
					LocalTenantID: 1,
					CheckClass:    sm.CheckClass_SCRIPTED,
					Duration:      59 * time.Second,
				},
				{
					LocalTenantID: 1,
					CheckClass:    sm.CheckClass_PROTOCOL,
					Duration:      130 * time.Second,
				},
				{
					LocalTenantID: 1,
					CheckClass:    sm.CheckClass_SCRIPTED,
					Duration:      60 * time.Second,
				},
				{
					LocalTenantID: 2,
					CheckClass:    sm.CheckClass_PROTOCOL,
					Duration:      45 * time.Second,
				},
				{
					LocalTenantID: 1,
					CheckClass:    sm.CheckClass_SCRIPTED,
					Duration:      65 * time.Second,
				},
				{
					LocalTenantID: 3,
					CheckClass:    sm.CheckClass_BROWSER,
					Duration:      65 * time.Second,
				},
				{
					LocalTenantID: 3,
					CheckClass:    sm.CheckClass_BROWSER,
					Duration:      30 * time.Second,
					CostAttributionLabels: []sm.CostAttributionLabel{
						{
							Name:  "env",
							Value: "prod",
						},
						{
							Name:  "team",
							Value: "__MISSING__",
						},
					},
				},
				{
					LocalTenantID: 3,
					CheckClass:    sm.CheckClass_BROWSER,
					Duration:      30 * time.Second,
					CostAttributionLabels: []sm.CostAttributionLabel{
						{
							Name:  "team",
							Value: "a",
						},
						{
							Name:  "env",
							Value: "__MISSING__",
						},
					},
				},
			},
			message: sm.RegionTelemetry{
				Instance: instance,
				RegionId: 1,
				Telemetry: []*sm.TenantTelemetry{
					{
						TenantId: 1,
						Telemetry: []*sm.CheckClassTelemetry{
							{
								CheckClass:            sm.CheckClass_PROTOCOL,
								Executions:            3,
								Duration:              249,
								SampledExecutions:     5,
								CostAttributionLabels: []sm.CostAttributionLabel{},
							},
							{
								CheckClass:            sm.CheckClass_SCRIPTED,
								Executions:            4,
								Duration:              214,
								SampledExecutions:     5,
								CostAttributionLabels: []sm.CostAttributionLabel{},
							},
						},
					},
					{
						TenantId: 2,
						Telemetry: []*sm.CheckClassTelemetry{
							{
								CheckClass:        sm.CheckClass_PROTOCOL,
								Executions:        1,
								Duration:          45,
								SampledExecutions: 1,

								CostAttributionLabels: []sm.CostAttributionLabel{},
							},
							{
								CheckClass:            sm.CheckClass_SCRIPTED,
								Executions:            2,
								Duration:              91,
								SampledExecutions:     3,
								CostAttributionLabels: []sm.CostAttributionLabel{},
							},
						},
					},
					{
						TenantId: 3,
						Telemetry: []*sm.CheckClassTelemetry{
							{
								CheckClass:            sm.CheckClass_BROWSER,
								Executions:            3,
								Duration:              156,
								SampledExecutions:     5,
								CostAttributionLabels: []sm.CostAttributionLabel{},
							},
							{
								CheckClass:        sm.CheckClass_BROWSER,
								Executions:        2,
								Duration:          60,
								SampledExecutions: 2,
								CostAttributionLabels: []sm.CostAttributionLabel{
									{
										Name:  "env",
										Value: "prod",
									},
									{
										Name:  "team",
										Value: "__MISSING__",
									},
								},
							},
							{
								CheckClass:        sm.CheckClass_BROWSER,
								Executions:        2,
								Duration:          60,
								SampledExecutions: 2,
								CostAttributionLabels: []sm.CostAttributionLabel{
									{
										Name:  "env",
										Value: "__MISSING__",
									},
									{
										Name:  "team",
										Value: "a",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Why bother with a copy? For the same reason the above data is not a
	// global variable: because we don't trust the caller to behave and not
	// modify the data.
	return testData{
		executions: append([]Execution{}, data[idx].executions...),
		message:    data[idx].message,
	}
}

func TestTenantPusher(t *testing.T) {
	t.Parallel()

	var (
		testPushRespOK = testPushResp{
			tr: &sm.PushTelemetryResponse{
				Status: &sm.Status{Code: sm.StatusCode_OK},
			},
		}
		testPushRespKO = testPushResp{
			tr: &sm.PushTelemetryResponse{
				Status: &sm.Status{Code: sm.StatusCode_INTERNAL_ERROR},
			},
		}
	)

	addExecutions := func(p *RegionPusher, executions []Execution) {
		for _, execution := range executions {
			p.AddExecution(execution)
		}
	}

	t.Run("should send telemetry data once", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			td, tc, pusher, _ := setupTest(t)

			t.Cleanup(td.shutdownAndWait)

			// Add some executions.
			addExecutions(pusher, getTestDataset(0).executions)

			// Set mock response for client.
			tc.rr = testPushRespOK

			// Tick.
			td.tickAndWait()

			// Verify sent data.
			tc.assert(t, getTestDataset(0).message)
		})
	})

	t.Run("should retry sending data once", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			td, tc, pusher, _ := setupTest(t)

			t.Cleanup(td.shutdownAndWait)

			// Add some executions.
			addExecutions(pusher, getTestDataset(0).executions)

			// Set mock response for client.
			tc.rr = testPushRespKO

			// Tick twice, one for initial push and one for retry.
			td.tickAndWait()
			td.tickAndWait()

			// Verify sent data.
			tc.assert(t, getTestDataset(0).message, getTestDataset(0).message)
		})
	})

	t.Run("should retry and send more data", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			td, tc, pusher, _ := setupTest(t)

			t.Cleanup(td.shutdownAndWait)

			// Add some executions.
			addExecutions(pusher, getTestDataset(0).executions)

			// Set KO mock response for client and tick once.
			tc.rr = testPushRespKO
			td.tickAndWait()

			// Send more executions.
			addExecutions(pusher, getTestDataset(1).executions)

			// Set OK mock response for client and tick again.
			tc.rr = testPushRespOK
			td.tickAndWait()

			// Verify sent data.
			tc.assert(t,
				getTestDataset(0).message, // First tick message
				getTestDataset(1).message, // First message retry with newly accumulated data
			)
		})
	})

	t.Run("should push on context done", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			td, tc, pusher, _ := setupTest(t)

			// Add some executions.
			addExecutions(pusher, getTestDataset(0).executions)

			// Set mock response for client.
			tc.rr = testPushRespKO

			// Tick once, which should make the push fail.
			td.tickAndWait()

			// Verify sent data.
			tc.assert(t, getTestDataset(0).message)

			// Send more executions.
			addExecutions(pusher, getTestDataset(1).executions)

			// Cancel the context, which should make the pusher send the currently accumulated data.
			td.shutdownAndWait()

			// Verify sent data on exit.
			tc.assert(t, getTestDataset(1).message)
		})
	})

	t.Run("should report push error", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			td, tc, pusher, metrics := setupTest(t)

			t.Cleanup(td.shutdownAndWait)

			// Setup test client to return err on push.
			tc.rr.err = errors.New("test error")

			// Add some executions.
			addExecutions(pusher, getTestDataset(0).executions)

			// Tick once, which should make the push fail
			td.tickAndWait()

			// Verify sent data.
			tc.assert(t, getTestDataset(0).message)

			// Verify error metric.
			//
			// td.tickAndWait() above ensures all goroutines have completed or are durably blocked.
			errsMetric := getMetricFromCollector(t, metrics.pushRequestsError)
			require.Equal(t, 1, int(*errsMetric.Counter.Value))
		})
	})

	t.Run("should report push error on unexpected status", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			td, tc, pusher, metrics := setupTest(t)

			t.Cleanup(td.shutdownAndWait)

			// Add some executions.
			addExecutions(pusher, getTestDataset(0).executions)

			// Set mock response for client with unexpected status code.
			tc.rr = testPushRespKO

			// Tick once, which should make the push fail.
			td.tickAndWait()

			// Verify sent data.
			tc.assert(t, getTestDataset(0).message)

			// Verify error metric.
			//
			// td.tickAndWait() above ensures all goroutines have completed or are durably blocked.
			errsMetric := getMetricFromCollector(t, metrics.pushRequestsError)
			require.Equal(t, 1, int(*errsMetric.Counter.Value))
		})
	})
}

type testTicker struct {
	c chan time.Time
}

func (t *testTicker) C() <-chan time.Time {
	return t.c
}

func (t *testTicker) Stop() {
	close(t.c)
}

type testPushResp struct {
	tr  *sm.PushTelemetryResponse
	err error
}

func setupTest(t *testing.T) (*testDriver, *testTelemetryClient, *RegionPusher, RegionMetrics) {
	// All setup happens within synctest bubble synctest will track all goroutines automatically.
	//
	// Do not use t.Context() here, as that context is derived from the main context, which is
	// created outside the bubble.
	testCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ticker := &testTicker{c: make(chan time.Time)}
	td := testDriver{cancel: cancel, ticker: ticker}
	tc := &testTelemetryClient{}
	logger := testhelper.Logger(t)
	metrics := RegionMetrics{
		pushRequestsActive:   prom.NewGauge(prom.GaugeOpts{}),
		pushRequestsDuration: prom.NewHistogram(prom.HistogramOpts{}),
		pushRequestsTotal:    prom.NewCounter(prom.CounterOpts{}),
		pushRequestsError:    prom.NewCounter(prom.CounterOpts{}),
		addExecutionDuration: prom.NewHistogram(prom.HistogramOpts{}),
	}

	// Pass a WaitGroup to maintain compatibility with the production code,
	// but synctest.Wait() will handle the synchronization.
	wg := new(sync.WaitGroup)

	pusher := NewRegionPusher(
		testCtx,
		1*time.Second,
		tc,
		logger,
		instance,
		regionID,
		metrics,
		WithTicker(ticker),
		WithWaitGroup(wg),
	)

	return &td, tc, pusher, metrics
}

type testDriver struct {
	cancel context.CancelFunc
	ticker *testTicker
}

// tickAndWait will tick the ticker once, so the push process starts, and wait for all goroutines to be durably blocked.
func (td *testDriver) tickAndWait() {
	td.ticker.c <- time.Now()
	synctest.Wait() // Wait for all goroutines to be durably blocked or complete.
}

// shutdownAndWait will cancel the context and wait for the pusher to finish.
func (td *testDriver) shutdownAndWait() {
	td.cancel()
	synctest.Wait() // Wait for pusher to finish its final push.
}

type testTelemetryClient struct {
	mu sync.Mutex

	rr testPushResp
	mm []sm.RegionTelemetry
}

func (tc *testTelemetryClient) PushTelemetry(
	ctx context.Context, in *sm.RegionTelemetry, opts ...grpc.CallOption,
) (*sm.PushTelemetryResponse, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.mm = append(tc.mm, *in)

	return tc.rr.tr, tc.rr.err
}

func (tc *testTelemetryClient) assert(t *testing.T, exp ...sm.RegionTelemetry) {
	t.Helper()

	defer func() {
		// Copy the remaining messages in the mm buffer to the front, and clip it to that size.
		rest := copy(tc.mm, tc.mm[len(exp):])
		tc.mm = tc.mm[:rest]
	}()

	for i, msg := range exp {
		assertInfoData(t, &msg, &tc.mm[i])
		assertRegionTelemetryData(t, &msg, &tc.mm[i])
	}
}

func assertInfoData(t *testing.T, exp, got *sm.RegionTelemetry) {
	t.Helper()
	require.Equal(t, exp.Instance, got.Instance, "instances should match")
	require.Equal(t, exp.RegionId, got.RegionId, "regions should match")
}

func assertRegionTelemetryData(t *testing.T, exp, got *sm.RegionTelemetry) {
	t.Helper()
	require.Equal(t, len(exp.Telemetry), len(got.Telemetry), "region telemetry length should match")

	// Because the message is built in the pusher by iterating a map, the
	// order is not deterministic, therefore we have to find each element
LOOP:
	for _, expTenantTele := range exp.Telemetry {
		for j, gotTenantTele := range got.Telemetry {
			if expTenantTele.TenantId == gotTenantTele.TenantId {
				assertTenantTelemetryData(t, expTenantTele, gotTenantTele)
				got.Telemetry = append(got.Telemetry[:j], got.Telemetry[j+1:]...)
				continue LOOP
			}
		}
		t.Fatalf("region telemetry not found: %v", expTenantTele)
	}
}

func assertTenantTelemetryData(t *testing.T, exp, got *sm.TenantTelemetry) {
	t.Helper()
	require.Equal(t, len(exp.Telemetry), len(got.Telemetry), "tenant telemetry length should match")

LOOP:
	for _, expTele := range exp.Telemetry {
		for j, gotTele := range got.Telemetry {
			if reflect.DeepEqual(expTele, gotTele) {
				got.Telemetry = append(got.Telemetry[:j], got.Telemetry[j+1:]...)
				continue LOOP
			}
		}
		t.Fatalf("tenant telemetry not found: %v", expTele)
	}
}

func getMetricFromCollector(t *testing.T, c prom.Collector) *prommodel.Metric {
	t.Helper()

	metricCh := make(chan prom.Metric)
	defer close(metricCh)
	go c.Collect(metricCh)
	metric := <-metricCh

	m := &prommodel.Metric{}
	require.NoError(t, metric.Write(m))

	return m
}
