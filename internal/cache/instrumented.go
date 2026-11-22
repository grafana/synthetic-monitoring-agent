package cache

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricNamespace = "sm_agent"
	metricSubsystem = "cache"

	opSet    = "set"
	opGet    = "get"
	opDelete = "delete"
	opFlush  = "flush"

	resultOK    = "ok"
	resultError = "error"
	resultHit   = "hit"
	resultMiss  = "miss"
)

// Metrics holds the Prometheus collectors shared by every instrumented cache.
// Create one per registry with NewMetrics and hand it to NewInstrumented.
type Metrics struct {
	ops      *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewMetrics creates and registers the cache metrics on reg:
//
//	sm_agent_cache_ops_total{backend,op,result}
//	sm_agent_cache_op_duration_seconds{backend,op}
//
// It panics if the collectors are already registered on reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		ops: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricNamespace,
			Subsystem: metricSubsystem,
			Name:      "ops_total",
			Help:      "Total number of cache operations by backend, operation and result.",
		}, []string{"backend", "op", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Subsystem: metricSubsystem,
			Name:      "op_duration_seconds",
			Help:      "Duration of cache operations in seconds by backend and operation.",
			// 0.5ms to ~256ms; callers budget about 100ms per operation.
			Buckets: prometheus.ExponentialBuckets(0.0005, 2, 10),
		}, []string{"backend", "op"}),
	}

	reg.MustRegister(m.ops, m.duration)

	return m
}

// opMetrics holds the resolved series for one (backend, op) pair so the hot
// path does no label lookups.
type opMetrics struct {
	duration prometheus.Observer
	results  map[string]prometheus.Counter
}

func (o opMetrics) observe(start time.Time, result string) {
	o.duration.Observe(time.Since(start).Seconds())
	o.results[result].Inc()
}

// instrumented decorates a Cache, counting and timing every operation.
type instrumented struct {
	inner Cache
	set   opMetrics
	get   opMetrics
	del   opMetrics
	flush opMetrics
}

var _ io.Closer = (*instrumented)(nil)

// NewInstrumented wraps inner so that every operation is counted in m under
// the given backend label. Return values are passed through unchanged. The
// returned Cache also implements io.Closer, forwarding to inner when it does.
//
// Every series is created up front so rate() queries see zeros instead of
// gaps before the first operation of each kind.
func NewInstrumented(inner Cache, backend string, m *Metrics) Cache {
	resolve := func(op string, results ...string) opMetrics {
		o := opMetrics{
			duration: m.duration.WithLabelValues(backend, op),
			results:  make(map[string]prometheus.Counter, len(results)),
		}

		for _, result := range results {
			o.results[result] = m.ops.WithLabelValues(backend, op, result)
		}

		return o
	}

	return &instrumented{
		inner: inner,
		set:   resolve(opSet, resultOK, resultError),
		get:   resolve(opGet, resultHit, resultMiss, resultError),
		del:   resolve(opDelete, resultOK, resultError),
		flush: resolve(opFlush, resultOK, resultError),
	}
}

func writeResult(err error) string {
	if err != nil {
		return resultError
	}

	return resultOK
}

func getResult(err error) string {
	switch {
	case err == nil:
		return resultHit
	case errors.Is(err, ErrCacheMiss):
		return resultMiss
	default:
		return resultError
	}
}

func (i *instrumented) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	start := time.Now()
	err := i.inner.Set(ctx, key, value, expiration)
	i.set.observe(start, writeResult(err))

	return err
}

func (i *instrumented) Get(ctx context.Context, key string, dest any) error {
	start := time.Now()
	err := i.inner.Get(ctx, key, dest)
	i.get.observe(start, getResult(err))

	return err
}

func (i *instrumented) Delete(ctx context.Context, key string) error {
	start := time.Now()
	err := i.inner.Delete(ctx, key)
	i.del.observe(start, writeResult(err))

	return err
}

func (i *instrumented) Flush(ctx context.Context) error {
	start := time.Now()
	err := i.inner.Flush(ctx)
	i.flush.observe(start, writeResult(err))

	return err
}

// Close forwards to the inner cache when it implements io.Closer.
func (i *instrumented) Close() error {
	if c, ok := i.inner.(io.Closer); ok {
		return c.Close()
	}

	return nil
}
