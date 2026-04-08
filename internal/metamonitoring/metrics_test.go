package metamonitoring

import (
	"context"
	"sync"
	"testing"
	"time"

	logproto "github.com/grafana/loki/pkg/push"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type capturedPayload struct {
	tenantID model.GlobalID
	metrics  []prompb.TimeSeries
}

type mockPublisher struct {
	mu       sync.Mutex
	payloads []capturedPayload
}

func (m *mockPublisher) Publish(p pusher.Payload) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payloads = append(m.payloads, capturedPayload{
		tenantID: p.Tenant(),
		metrics:  p.Metrics(),
	})
}

func (m *mockPublisher) getPayloads() []capturedPayload {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]capturedPayload, len(m.payloads))
	copy(cp, m.payloads)
	return cp
}

func newTestHandler(t *testing.T, registry *prometheus.Registry, tenantID model.GlobalID) (*metricsHandler, *mockPublisher) {
	t.Helper()
	pub := &mockPublisher{}
	handler, err := NewHandler(HandlerOpts{
		Logger:    zerolog.New(zerolog.NewTestWriter(t)),
		Registry:  registry,
		Publisher: pub,
		TenantID:  tenantID,
	})
	require.NoError(t, err)
	return handler, pub
}

func TestReportUsage(t *testing.T) {
	t.Run("publishes metrics with correct tenant ID", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		counter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_requests_total",
			Help: "Test counter",
		})
		registry.MustRegister(counter)
		counter.Add(5)

		handler, pub := newTestHandler(t, registry, 42)

		err := handler.reportUsage()
		require.NoError(t, err)

		payloads := pub.getPayloads()
		require.Len(t, payloads, 1)
		require.Equal(t, model.GlobalID(42), payloads[0].tenantID)
	})

	t.Run("publishes counter value", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		counter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_requests_total",
			Help: "Test counter",
		})
		registry.MustRegister(counter)
		counter.Add(7)

		handler, pub := newTestHandler(t, registry, 1)

		err := handler.reportUsage()
		require.NoError(t, err)

		payloads := pub.getPayloads()
		require.Len(t, payloads, 1)
		require.Len(t, payloads[0].metrics, 1)

		ts := payloads[0].metrics[0]
		require.Contains(t, ts.Labels, prompb.Label{Name: "__name__", Value: "test_requests_total"})
		require.Len(t, ts.Samples, 1)
		require.Equal(t, float64(7), ts.Samples[0].Value)
	})

	t.Run("publishes gauge value", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		gauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_temperature",
			Help: "Test gauge",
		})
		registry.MustRegister(gauge)
		gauge.Set(36.6)

		handler, pub := newTestHandler(t, registry, 1)

		err := handler.reportUsage()
		require.NoError(t, err)

		payloads := pub.getPayloads()
		require.Len(t, payloads, 1)
		require.Len(t, payloads[0].metrics, 1)

		ts := payloads[0].metrics[0]
		require.Contains(t, ts.Labels, prompb.Label{Name: "__name__", Value: "test_temperature"})
		require.InDelta(t, 36.6, ts.Samples[0].Value, 0.001)
	})

	t.Run("preserves metric labels", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		counter := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_requests_total",
			Help: "Test counter with labels",
		}, []string{"method", "status"})
		registry.MustRegister(counter)
		counter.WithLabelValues("GET", "200").Add(3)

		handler, pub := newTestHandler(t, registry, 1)

		err := handler.reportUsage()
		require.NoError(t, err)

		payloads := pub.getPayloads()
		require.Len(t, payloads, 1)
		require.Len(t, payloads[0].metrics, 1)

		ts := payloads[0].metrics[0]
		require.Contains(t, ts.Labels, prompb.Label{Name: "__name__", Value: "test_requests_total"})
		require.Contains(t, ts.Labels, prompb.Label{Name: "method", Value: "GET"})
		require.Contains(t, ts.Labels, prompb.Label{Name: "status", Value: "200"})
	})

	t.Run("skips publish when registry is empty", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		handler, pub := newTestHandler(t, registry, 1)

		err := handler.reportUsage()
		require.NoError(t, err)

		payloads := pub.getPayloads()
		require.Empty(t, payloads)
	})

	t.Run("skips histogram metrics", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "test_duration_seconds",
			Help:    "Test histogram",
			Buckets: prometheus.DefBuckets,
		})
		registry.MustRegister(histogram)
		histogram.Observe(0.5)

		handler, pub := newTestHandler(t, registry, 1)

		err := handler.reportUsage()
		require.NoError(t, err)

		payloads := pub.getPayloads()
		require.Empty(t, payloads)
	})

	t.Run("skips summary metrics", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		summary := prometheus.NewSummary(prometheus.SummaryOpts{
			Name: "test_latency_seconds",
			Help: "Test summary",
		})
		registry.MustRegister(summary)
		summary.Observe(0.3)

		handler, pub := newTestHandler(t, registry, 1)

		err := handler.reportUsage()
		require.NoError(t, err)

		payloads := pub.getPayloads()
		require.Empty(t, payloads)
	})

	t.Run("publishes multiple metrics in one payload", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		counter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_counter",
			Help: "Test counter",
		})
		gauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_gauge",
			Help: "Test gauge",
		})
		registry.MustRegister(counter, gauge)
		counter.Add(1)
		gauge.Set(2)

		handler, pub := newTestHandler(t, registry, 1)

		err := handler.reportUsage()
		require.NoError(t, err)

		payloads := pub.getPayloads()
		require.Len(t, payloads, 1)
		require.Len(t, payloads[0].metrics, 2)
	})

	t.Run("payload streams are nil without log buffer", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		counter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_counter",
			Help: "Test counter",
		})
		registry.MustRegister(counter)
		counter.Add(1)

		handler, pub := newTestHandler(t, registry, 1)

		// Capture the raw payload through a wrapping publisher.
		var rawPayload pusher.Payload
		wrapper := &payloadCapture{inner: pub, captured: &rawPayload}
		handler.publisher = wrapper

		err := handler.reportUsage()
		require.NoError(t, err)
		require.NotNil(t, rawPayload)
		require.Nil(t, rawPayload.Streams())
	})

	t.Run("payload includes drained log streams", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		counter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_counter",
			Help: "Test counter",
		})
		registry.MustRegister(counter)
		counter.Add(1)

		logBuf := NewRingBuffer(100)
		logBuf.Append(logproto.Stream{
			Labels:  `{source="test"}`,
			Entries: []logproto.Entry{{Line: "test log line"}},
		})

		pub := &mockPublisher{}
		handler, err := NewHandler(HandlerOpts{
			Logger:    zerolog.New(zerolog.NewTestWriter(t)),
			Registry:  registry,
			Publisher: pub,
			TenantID:  1,
			LogBuffer: logBuf,
		})
		require.NoError(t, err)

		var rawPayload pusher.Payload
		wrapper := &payloadCapture{inner: pub, captured: &rawPayload}
		handler.publisher = wrapper

		err = handler.reportUsage()
		require.NoError(t, err)
		require.NotNil(t, rawPayload)
		require.Len(t, rawPayload.Streams(), 1)
		require.Equal(t, `{source="test"}`, rawPayload.Streams()[0].Labels)
		require.Equal(t, "test log line", rawPayload.Streams()[0].Entries[0].Line)
	})

	t.Run("publishes streams even without metrics", func(t *testing.T) {
		registry := prometheus.NewRegistry() // empty

		logBuf := NewRingBuffer(100)
		logBuf.Append(logproto.Stream{
			Labels:  `{source="test"}`,
			Entries: []logproto.Entry{{Line: "log only"}},
		})

		pub := &mockPublisher{}
		handler, err := NewHandler(HandlerOpts{
			Logger:    zerolog.New(zerolog.NewTestWriter(t)),
			Registry:  registry,
			Publisher: pub,
			TenantID:  1,
			LogBuffer: logBuf,
		})
		require.NoError(t, err)

		err = handler.reportUsage()
		require.NoError(t, err)

		payloads := pub.getPayloads()
		require.Len(t, payloads, 1)
	})

	t.Run("timestamp is set on samples", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		gauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_gauge",
			Help: "Test gauge",
		})
		registry.MustRegister(gauge)
		gauge.Set(1)

		handler, pub := newTestHandler(t, registry, 1)

		before := time.Now().UnixNano() / 1e6
		err := handler.reportUsage()
		require.NoError(t, err)
		after := time.Now().UnixNano() / 1e6

		payloads := pub.getPayloads()
		require.Len(t, payloads, 1)

		stamp := payloads[0].metrics[0].Samples[0].Timestamp
		require.GreaterOrEqual(t, stamp, before)
		require.LessOrEqual(t, stamp, after)
	})
}

func TestRun(t *testing.T) {
	t.Run("publishes on each tick", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		counter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_tick_counter",
			Help: "Test counter",
		})
		registry.MustRegister(counter)
		counter.Add(1)

		pub := &mockPublisher{}
		handler, err := NewHandler(HandlerOpts{
			Logger:    zerolog.New(zerolog.NewTestWriter(t)),
			Registry:  registry,
			Publisher: pub,
			TenantID:  1,
			Interval:  50 * time.Millisecond,
		})
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 175*time.Millisecond)
		defer cancel()

		err = handler.Run(ctx)
		require.NoError(t, err)

		payloads := pub.getPayloads()
		require.GreaterOrEqual(t, len(payloads), 2)
		require.LessOrEqual(t, len(payloads), 4)
	})

	t.Run("stops on context cancellation", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		pub := &mockPublisher{}
		handler, err := NewHandler(HandlerOpts{
			Logger:    zerolog.New(zerolog.NewTestWriter(t)),
			Registry:  registry,
			Publisher: pub,
			TenantID:  1,
			Interval:  time.Hour, // very long — should not tick
		})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		done := make(chan error, 1)
		go func() {
			done <- handler.Run(ctx)
		}()

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("Run did not return after context cancellation")
		}

		payloads := pub.getPayloads()
		require.Empty(t, payloads)
	})

	t.Run("defaults interval to 1 minute", func(t *testing.T) {
		handler, _ := newTestHandler(t, prometheus.NewRegistry(), 1)
		require.Equal(t, time.Minute, handler.interval)
	})
}

// payloadCapture wraps a publisher and captures the raw Payload for interface assertions.
type payloadCapture struct {
	inner    pusher.Publisher
	captured *pusher.Payload
}

func (p *payloadCapture) Publish(payload pusher.Payload) {
	*p.captured = payload
	p.inner.Publish(payload)
}
