package telemetry

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"

	prom "github.com/prometheus/client_golang/prometheus"
	prommodel "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog"
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
			},
			message: sm.RegionTelemetry{
				Instance: instance,
				RegionId: 1,
				Telemetry: []*sm.TenantTelemetry{
					{
						TenantId: 1,
						Telemetry: []*sm.CheckClassTelemetry{
							{
								CheckClass:        sm.CheckClass_PROTOCOL,
								Executions:        2,
								Duration:          119,
								SampledExecutions: 2,
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
							},
						},
					},
					{
						TenantId: 3,
						Telemetry: []*sm.CheckClassTelemetry{
							{
								CheckClass:        sm.CheckClass_BROWSER,
								Executions:        2,
								Duration:          91,
								SampledExecutions: 3,
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
			},
			message: sm.RegionTelemetry{
				Instance: instance,
				RegionId: 1,
				Telemetry: []*sm.TenantTelemetry{
					{
						TenantId: 1,
						Telemetry: []*sm.CheckClassTelemetry{
							{
								CheckClass:        sm.CheckClass_PROTOCOL,
								Executions:        3,
								Duration:          249,
								SampledExecutions: 5,
							},
							{
								CheckClass:        sm.CheckClass_SCRIPTED,
								Executions:        4,
								Duration:          214,
								SampledExecutions: 5,
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
							},
							{
								CheckClass:        sm.CheckClass_SCRIPTED,
								Executions:        2,
								Duration:          91,
								SampledExecutions: 3,
							},
						},
					},
					{
						TenantId: 3,
						Telemetry: []*sm.CheckClassTelemetry{
							{
								CheckClass:        sm.CheckClass_BROWSER,
								Executions:        3,   // 2 + 1
								Duration:          156, // 61 + 30 + 65
								SampledExecutions: 5,   // 2 + 1 + 2
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

var m = RegionMetrics{
	pushRequestsActive:   prom.NewGauge(prom.GaugeOpts{}),
	pushRequestsDuration: prom.NewHistogram(prom.HistogramOpts{}),
	pushRequestsTotal:    prom.NewCounter(prom.CounterOpts{}),
	pushRequestsError:    prom.NewCounter(prom.CounterOpts{}),
	addExecutionDuration: prom.NewHistogram(prom.HistogramOpts{}),
}

func TestTenantPusher(t *testing.T) {
	var (
		// This time span is passed to the tenant constructor, but it's ignored
		// because we are overriding the ticker with one that we can control
		timeSpan = 1 * time.Second

		logger = zerolog.Nop()

		// Because the push happens in a separate goroutine, we use a waitgroup
		// to wait for the mock push client to finish before verifying the data
		wg         = &sync.WaitGroup{}
		testClient = &testTelemetryClient{wg: wg}

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

	// tickAndWait will tick the ticker once, so the push
	// process starts, and wait for the push client to finish
	tickAndWait := func(ticker *testTicker) {
		wg.Add(1)
		defer wg.Wait()
		ticker.c <- time.Now()
	}

	// waitForShutdown will cancel the context passed to the
	// tenant pusher and wait for it to finish its work
	shutdownAndWait := func(cancel context.CancelFunc) {
		defer wg.Wait()
		// The pusher will send the current accumulated
		// data before exiting
		wg.Add(1)

		cancel()
	}

	resetTestClient := func() {
		testClient = &testTelemetryClient{wg: wg}
	}

	t.Run("should send telemetry data once", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		t.Cleanup(func() {
			shutdownAndWait(cancel)
			resetTestClient()
		})

		ticker := &testTicker{
			c: make(chan time.Time),
		}
		var opt withTicker = ticker

		pusher := NewRegionPusher(ctx, timeSpan, testClient, logger, instance, regionID, m, opt)

		// Add some executions
		addExecutions(pusher, getTestDataset(0).executions)

		// Set mock response for client
		testClient.rr = testPushRespOK

		// Tick
		tickAndWait(ticker)

		// Verify sent data
		testClient.assert(t, getTestDataset(0).message)
	})

	t.Run("should retry sending data once", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		t.Cleanup(func() {
			shutdownAndWait(cancel)
			resetTestClient()
		})

		ticker := &testTicker{
			c: make(chan time.Time),
		}
		var opt withTicker = ticker

		pusher := NewRegionPusher(ctx, timeSpan, testClient, logger, instance, regionID, m, opt)

		// Add some executions
		addExecutions(pusher, getTestDataset(0).executions)

		// Set mock response for client
		testClient.rr = testPushRespKO

		// Tick twice, one for initial push and one for retry
		tickAndWait(ticker)
		tickAndWait(ticker)

		// Verify sent data
		testClient.assert(t, getTestDataset(0).message, getTestDataset(0).message)
	})

	t.Run("should retry and send more data", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		t.Cleanup(func() {
			shutdownAndWait(cancel)
			resetTestClient()
		})

		ticker := &testTicker{
			c: make(chan time.Time),
		}
		var opt withTicker = ticker

		pusher := NewRegionPusher(ctx, timeSpan, testClient, logger, instance, regionID, m, opt)

		// Add some executions
		addExecutions(pusher, getTestDataset(0).executions)

		// Set KO mock response for client and tick once
		testClient.rr = testPushRespKO
		tickAndWait(ticker)

		// Send more executions
		addExecutions(pusher, getTestDataset(1).executions)

		// Set OK mock response for client and tick again
		testClient.rr = testPushRespOK
		tickAndWait(ticker)

		// Verify sent data
		testClient.assert(t,
			getTestDataset(0).message, // First tick message
			getTestDataset(1).message, // First message retry with newly accumulated data
		)
	})

	t.Run("should push on context done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		ticker := &testTicker{
			c: make(chan time.Time),
		}
		var opt withTicker = ticker

		pusher := NewRegionPusher(ctx, timeSpan, testClient, logger, instance, regionID, m, opt)

		// Add some executions
		addExecutions(pusher, getTestDataset(0).executions)

		// Set mock response for client
		testClient.rr = testPushRespKO

		// Tick once, which should make the push fail
		tickAndWait(ticker)

		// Verify sent data
		testClient.assert(t, getTestDataset(0).message)

		// Send more executions
		addExecutions(pusher, getTestDataset(1).executions)

		// Cancel the context
		// Which should make the pusher send
		// the currently accumulated data
		shutdownAndWait(cancel)

		// Verify sent data on exit
		testClient.assert(t, getTestDataset(1).message)
	})

	t.Run("should report push error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		t.Cleanup(func() {
			shutdownAndWait(cancel)
			resetTestClient()
		})

		ticker := &testTicker{
			c: make(chan time.Time),
		}
		var opt withTicker = ticker

		metrics := RegionMetrics{
			pushRequestsActive:   prom.NewGauge(prom.GaugeOpts{}),
			pushRequestsDuration: prom.NewHistogram(prom.HistogramOpts{}),
			pushRequestsTotal:    prom.NewCounter(prom.CounterOpts{}),
			pushRequestsError:    prom.NewCounter(prom.CounterOpts{}),
			addExecutionDuration: prom.NewHistogram(prom.HistogramOpts{}),
		}

		// Setup test client to return err on push
		testClient.rr.err = errors.New("test error")

		pusher := NewRegionPusher(ctx, timeSpan, testClient, logger, instance, regionID, metrics, opt)

		// Add some executions
		addExecutions(pusher, getTestDataset(0).executions)

		// Tick once, which should make the push fail
		tickAndWait(ticker)

		// Verify sent data
		testClient.assert(t, getTestDataset(0).message)

		// Verify error metric
		errsMetric := getMetricFromCollector(t, metrics.pushRequestsError)
		require.Equal(t, 1, int(*errsMetric.Counter.Value))
	})

	t.Run("should report push error on unexpected status", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		t.Cleanup(func() {
			shutdownAndWait(cancel)
			resetTestClient()
		})

		ticker := &testTicker{
			c: make(chan time.Time),
		}
		var opt withTicker = ticker

		metrics := RegionMetrics{
			pushRequestsActive:   prom.NewGauge(prom.GaugeOpts{}),
			pushRequestsDuration: prom.NewHistogram(prom.HistogramOpts{}),
			pushRequestsTotal:    prom.NewCounter(prom.CounterOpts{}),
			pushRequestsError:    prom.NewCounter(prom.CounterOpts{}),
			addExecutionDuration: prom.NewHistogram(prom.HistogramOpts{}),
		}

		pusher := NewRegionPusher(ctx, timeSpan, testClient, logger, instance, regionID, metrics, opt)

		// Add some executions
		addExecutions(pusher, getTestDataset(0).executions)

		// Set mock response for client
		// with unexpected status code
		testClient.rr = testPushRespKO

		// Tick once, which should make the push fail
		tickAndWait(ticker)

		// Verify sent data
		testClient.assert(t, getTestDataset(0).message)

		// Verify error metric
		errsMetric := getMetricFromCollector(t, metrics.pushRequestsError)
		require.Equal(t, 1, int(*errsMetric.Counter.Value))
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

type testTelemetryClient struct {
	mu sync.Mutex
	wg *sync.WaitGroup

	rr testPushResp
	mm []sm.RegionTelemetry
}

func (tc *testTelemetryClient) PushTelemetry(
	ctx context.Context, in *sm.RegionTelemetry, opts ...grpc.CallOption,
) (*sm.PushTelemetryResponse, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	defer tc.wg.Done()

	tc.mm = append(tc.mm, *in)

	return tc.rr.tr, tc.rr.err
}

func (tc *testTelemetryClient) assert(t *testing.T, exp ...sm.RegionTelemetry) {
	t.Helper()

	defer func() {
		// reslice got messages moving forward for each one verified
		// so these are not taken into account if assert is called again
		tc.mm = tc.mm[len(exp):]
	}()
	for i, expM := range exp {
		assertInfoData(t, &expM, &tc.mm[i])
		assertRegionTelemetryData(t, &expM, &tc.mm[i])
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
