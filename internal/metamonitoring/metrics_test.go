package metamonitoring

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

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

func newTestHandler(t *testing.T, registry *prometheus.Registry, tenantID int64) (Handler, *mockPublisher) {
	t.Helper()
	pub := &mockPublisher{}
	handler := NewHandler(HandlerOpts{
		Logger:    zerolog.New(zerolog.NewTestWriter(t)),
		Registry:  registry,
		Publisher: pub,
		TenantID:  tenantID,
	})
	require.NotNil(t, handler)
	return handler, pub
}

func TestReportUsage(t *testing.T) {
	t.Run("publishes metrics with correct tenant ID", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			counter := prometheus.NewCounter(prometheus.CounterOpts{
				Name: "test_requests_total",
				Help: "Test counter",
			})
			registry.MustRegister(counter)
			counter.Add(5)

			handler, pub := newTestHandler(t, registry, 42)
			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				synctest.Wait()
			}()

			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("hander.Run: %v", err)
				}
			}()
			time.Sleep(defaultInterval)
			synctest.Wait()

			payloads := pub.getPayloads()
			require.Len(t, payloads, 1)
			require.Equal(t, model.GlobalID(42), payloads[0].tenantID)
		})
	})

	t.Run("publishes counter value", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			counter := prometheus.NewCounter(prometheus.CounterOpts{
				Name: "test_requests_total",
				Help: "Test counter",
			})
			registry.MustRegister(counter)
			counter.Add(7)

			handler, pub := newTestHandler(t, registry, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				synctest.Wait()
			}()

			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("hander.Run: %v", err)
				}
			}()
			time.Sleep(defaultInterval)
			synctest.Wait()

			payloads := pub.getPayloads()
			require.Len(t, payloads, 1)
			require.Len(t, payloads[0].metrics, 1)

			ts := payloads[0].metrics[0]
			require.Contains(t, ts.Labels, prompb.Label{Name: "__name__", Value: "test_requests_total"})
			require.Len(t, ts.Samples, 1)
			require.Equal(t, float64(7), ts.Samples[0].Value)
		})
	})

	t.Run("publishes gauge value", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			gauge := prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "test_temperature",
				Help: "Test gauge",
			})
			registry.MustRegister(gauge)
			gauge.Set(36.6)

			handler, pub := newTestHandler(t, registry, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				synctest.Wait()
			}()

			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("hander.Run: %v", err)
				}
			}()
			time.Sleep(defaultInterval)
			synctest.Wait()

			payloads := pub.getPayloads()
			require.Len(t, payloads, 1)
			require.Len(t, payloads[0].metrics, 1)

			ts := payloads[0].metrics[0]
			require.Contains(t, ts.Labels, prompb.Label{Name: "__name__", Value: "test_temperature"})
			require.InDelta(t, 36.6, ts.Samples[0].Value, 0.001)
		})
	})

	t.Run("preserves metric labels", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			counter := prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: "test_requests_total",
				Help: "Test counter with labels",
			}, []string{"method", "status"})
			registry.MustRegister(counter)
			counter.WithLabelValues("GET", "200").Add(3)

			handler, pub := newTestHandler(t, registry, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				synctest.Wait()
			}()

			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("hander.Run: %v", err)
				}
			}()
			time.Sleep(defaultInterval)
			synctest.Wait()

			payloads := pub.getPayloads()
			require.Len(t, payloads, 1)
			require.Len(t, payloads[0].metrics, 1)

			ts := payloads[0].metrics[0]
			require.Contains(t, ts.Labels, prompb.Label{Name: "__name__", Value: "test_requests_total"})
			require.Contains(t, ts.Labels, prompb.Label{Name: "method", Value: "GET"})
			require.Contains(t, ts.Labels, prompb.Label{Name: "status", Value: "200"})
		})
	})

	t.Run("skips publish when registry is empty", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			handler, pub := newTestHandler(t, registry, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				synctest.Wait()
			}()

			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("hander.Run: %v", err)
				}
			}()
			time.Sleep(defaultInterval)
			synctest.Wait()

			payloads := pub.getPayloads()
			require.Empty(t, payloads)
		})
	})

	t.Run("skips histogram metrics", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:    "test_duration_seconds",
				Help:    "Test histogram",
				Buckets: prometheus.DefBuckets,
			})
			registry.MustRegister(histogram)
			histogram.Observe(0.5)

			handler, pub := newTestHandler(t, registry, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				synctest.Wait()
			}()

			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("hander.Run: %v", err)
				}
			}()
			time.Sleep(defaultInterval)
			synctest.Wait()

			payloads := pub.getPayloads()
			require.Empty(t, payloads)
		})
	})

	t.Run("skips summary metrics", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			summary := prometheus.NewSummary(prometheus.SummaryOpts{
				Name: "test_latency_seconds",
				Help: "Test summary",
			})
			registry.MustRegister(summary)
			summary.Observe(0.3)

			handler, pub := newTestHandler(t, registry, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				synctest.Wait()
			}()

			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("hander.Run: %v", err)
				}
			}()
			time.Sleep(defaultInterval)
			synctest.Wait()

			payloads := pub.getPayloads()
			require.Empty(t, payloads)
		})
	})

	t.Run("publishes multiple metrics in one payload", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
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
			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				synctest.Wait()
			}()

			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("hander.Run: %v", err)
				}
			}()
			time.Sleep(defaultInterval)
			synctest.Wait()

			payloads := pub.getPayloads()
			require.Len(t, payloads, 1)
			require.Len(t, payloads[0].metrics, 2)
		})
	})

	t.Run("payload streams are nil", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			counter := prometheus.NewCounter(prometheus.CounterOpts{
				Name: "test_counter",
				Help: "Test counter",
			})
			registry.MustRegister(counter)
			counter.Add(1)

			pub := &mockPublisher{}
			var rawPayload pusher.Payload
			wrapper := &payloadCapture{inner: pub, captured: &rawPayload}

			handler := NewHandler(HandlerOpts{
				Logger:    zerolog.New(zerolog.NewTestWriter(t)),
				Registry:  registry,
				Publisher: wrapper,
				TenantID:  1,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				synctest.Wait()
			}()

			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("hander.Run: %v", err)
				}
			}()
			time.Sleep(defaultInterval)
			synctest.Wait()

			require.NotNil(t, rawPayload)
			require.Nil(t, rawPayload.Streams())
		})
	})

	t.Run("timestamp is set on samples", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			gauge := prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "test_gauge",
				Help: "Test gauge",
			})
			registry.MustRegister(gauge)
			gauge.Set(1)

			handler, pub := newTestHandler(t, registry, 1)

			before := time.Now().UnixNano() / 1e6

			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				synctest.Wait()
			}()

			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("hander.Run: %v", err)
				}
			}()
			time.Sleep(defaultInterval)
			synctest.Wait()

			after := time.Now().UnixNano() / 1e6

			payloads := pub.getPayloads()
			require.Len(t, payloads, 1)

			stamp := payloads[0].metrics[0].Samples[0].Timestamp
			require.GreaterOrEqual(t, stamp, before)
			require.LessOrEqual(t, stamp, after)
		})
	})
}

func TestRun(t *testing.T) {
	t.Run("handler returns an error if no tenant is passed to the probeTenantCh", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			probeTenantCh := make(chan int64, 1)
			handler := NewHandler(HandlerOpts{
				Logger:        zerolog.New(zerolog.NewTestWriter(t)),
				Registry:      prometheus.NewPedanticRegistry(),
				Publisher:     &mockPublisher{},
				ProbeTenantCh: probeTenantCh,
			})
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := handler.Run(ctx)
			require.ErrorIs(t, err, errTenantTimeout)
		})
	})

	t.Run("gets the tenantID from the channel if the TenantID is not set", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			probeTenantCh := make(chan int64, 1)
			handler := NewHandler(HandlerOpts{
				Logger:        zerolog.New(zerolog.NewTestWriter(t)),
				Registry:      prometheus.NewPedanticRegistry(),
				Publisher:     &mockPublisher{},
				ProbeTenantCh: probeTenantCh,
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				synctest.Wait()
			}()
			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("handler.Run: %v", err)
				}
			}()
			probeTenantCh <- 1
		})
	})
	t.Run("publishes on each tick", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			counter := prometheus.NewCounter(prometheus.CounterOpts{
				Name: "test_tick_counter",
				Help: "Test counter",
			})
			registry.MustRegister(counter)
			counter.Add(1)

			pub := &mockPublisher{}
			handler := NewHandler(HandlerOpts{
				Logger:    zerolog.New(zerolog.NewTestWriter(t)),
				Registry:  registry,
				Publisher: pub,
				TenantID:  1,
				Interval:  50 * time.Millisecond,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				synctest.Wait()
			}()

			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("hander.Run: %v", err)
				}
			}()

			// Advance 3 ticks exactly.
			time.Sleep(50 * time.Millisecond)
			synctest.Wait()
			time.Sleep(50 * time.Millisecond)
			synctest.Wait()
			time.Sleep(50 * time.Millisecond)
			synctest.Wait()

			payloads := pub.getPayloads()
			require.Len(t, payloads, 3)
		})
	})

	t.Run("stops on context cancellation", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			pub := &mockPublisher{}
			handler := NewHandler(HandlerOpts{
				Logger:    zerolog.New(zerolog.NewTestWriter(t)),
				Registry:  registry,
				Publisher: pub,
				TenantID:  1,
				Interval:  time.Hour,
			})

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			go func() {
				if err := handler.Run(ctx); err != nil {
					t.Errorf("hander.Run: %v", err)
				}
			}()
			synctest.Wait()

			payloads := pub.getPayloads()
			require.Empty(t, payloads)
		})
	})

	t.Run("defaults interval to 1 minute", func(t *testing.T) {
		handler, _ := newTestHandler(t, prometheus.NewRegistry(), 1)
		require.Equal(t, time.Minute, handler.(*metricsHandler).interval)
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
