package cache

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

var errFakeCache = errors.New("fake cache failure")

// failingCache returns errFakeCache from every operation.
type failingCache struct{}

var _ Cache = failingCache{}

func (failingCache) Set(context.Context, string, any, time.Duration) error { return errFakeCache }
func (failingCache) Get(context.Context, string, any) error                { return errFakeCache }
func (failingCache) Delete(context.Context, string) error                  { return errFakeCache }
func (failingCache) Flush(context.Context) error                           { return errFakeCache }

// closableCache records whether Close was called.
type closableCache struct {
	Cache
	closed bool
}

func (c *closableCache) Close() error {
	c.closed = true

	return nil
}

func TestInstrumentedClose(t *testing.T) {
	m := NewMetrics(prometheus.NewPedanticRegistry())

	t.Run("forwards Close to a closable inner", func(t *testing.T) {
		inner := &closableCache{Cache: NewNoop(testLogger())}
		cc := NewInstrumented(inner, "fake", m)

		closer, ok := cc.(io.Closer)
		require.True(t, ok)
		require.NoError(t, closer.Close())
		require.True(t, inner.closed)
	})

	t.Run("no-op for an inner without Close", func(t *testing.T) {
		cc := NewInstrumented(failingCache{}, "fake", m)

		closer, ok := cc.(io.Closer)
		require.True(t, ok)
		require.NoError(t, closer.Close())
	})

	t.Run("redis client closes through the Cache value", func(t *testing.T) {
		s := miniredis.RunT(t)

		client, err := NewRedisClient(t.Context(), RedisConfig{Addrs: []string{s.Addr()}, Logger: testLogger()})
		require.NoError(t, err)

		cc := NewInstrumented(client, "redis", m)

		closer, ok := cc.(io.Closer)
		require.True(t, ok)
		require.NoError(t, closer.Close())

		var result string

		require.Error(t, client.Get(t.Context(), "k", &result))
	})
}

func opsCount(t *testing.T, m *Metrics, backend, op, result string) float64 {
	t.Helper()

	return testutil.ToFloat64(m.ops.WithLabelValues(backend, op, result))
}

func durationCount(t *testing.T, m *Metrics, backend, op string) uint64 {
	t.Helper()

	var metric dto.Metric

	observer, ok := m.duration.WithLabelValues(backend, op).(prometheus.Histogram)
	require.True(t, ok)
	require.NoError(t, observer.Write(&metric))

	return metric.GetHistogram().GetSampleCount()
}

func TestNewMetrics(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	m := NewMetrics(reg)

	cc := NewInstrumented(NewNoop(testLogger()), "noop", m)
	require.Implements(t, (*Cache)(nil), cc)

	t.Run("exposes the expected metric names", func(t *testing.T) {
		mfs, err := reg.Gather()
		require.NoError(t, err)

		var names []string
		for _, mf := range mfs {
			names = append(names, mf.GetName())
		}

		require.ElementsMatch(t, []string{
			"sm_agent_cache_ops_total",
			"sm_agent_cache_op_duration_seconds",
		}, names)
	})

	t.Run("passes metric linting", func(t *testing.T) {
		problems, err := testutil.GatherAndLint(reg)
		require.NoError(t, err)
		require.Empty(t, problems)
	})

	t.Run("every label combination exists before any operation", func(t *testing.T) {
		// 3 write ops x {ok,error} + get x {hit,miss,error}
		require.Equal(t, 9, testutil.CollectAndCount(m.ops))
		require.Equal(t, 4, testutil.CollectAndCount(m.duration))
	})

	t.Run("histogram buckets span 0.5ms to 256ms", func(t *testing.T) {
		var metric dto.Metric

		observer, ok := m.duration.WithLabelValues("noop", "get").(prometheus.Histogram)
		require.True(t, ok)
		require.NoError(t, observer.Write(&metric))

		buckets := metric.GetHistogram().GetBucket()
		require.Len(t, buckets, 10)
		require.InDelta(t, 0.0005, buckets[0].GetUpperBound(), 1e-9)
		require.InDelta(t, 0.256, buckets[9].GetUpperBound(), 1e-9)
	})

	t.Run("registering twice on the same registry panics", func(t *testing.T) {
		require.Panics(t, func() { NewMetrics(reg) })
	})
}

func TestInstrumentedNoop(t *testing.T) {
	m := NewMetrics(prometheus.NewPedanticRegistry())
	cc := NewInstrumented(NewNoop(testLogger()), "noop", m)
	ctx := t.Context()

	var result string

	require.ErrorIs(t, cc.Get(ctx, "k", &result), ErrCacheMiss)
	require.Equal(t, 1.0, opsCount(t, m, "noop", "get", "miss"))
	require.Equal(t, 0.0, opsCount(t, m, "noop", "get", "hit"))
	require.Equal(t, 0.0, opsCount(t, m, "noop", "get", "error"))
	require.Equal(t, uint64(1), durationCount(t, m, "noop", "get"))

	require.NoError(t, cc.Set(ctx, "k", "v", time.Minute))
	require.Equal(t, 1.0, opsCount(t, m, "noop", "set", "ok"))
	require.Equal(t, uint64(1), durationCount(t, m, "noop", "set"))

	require.NoError(t, cc.Delete(ctx, "k"))
	require.Equal(t, 1.0, opsCount(t, m, "noop", "delete", "ok"))
	require.Equal(t, uint64(1), durationCount(t, m, "noop", "delete"))

	require.NoError(t, cc.Flush(ctx))
	require.Equal(t, 1.0, opsCount(t, m, "noop", "flush", "ok"))
	require.Equal(t, uint64(1), durationCount(t, m, "noop", "flush"))
}

func TestInstrumentedRedis(t *testing.T) {
	_, client := newTestRedis(t)
	m := NewMetrics(prometheus.NewPedanticRegistry())
	cc := NewInstrumented(client, "redis", m)
	ctx := t.Context()
	value := TestStruct{ID: 7, Name: "instrumented", Tags: []string{"x"}}

	var result TestStruct

	require.ErrorIs(t, cc.Get(ctx, "k", &result), ErrCacheMiss)
	require.Equal(t, 1.0, opsCount(t, m, "redis", "get", "miss"))

	require.NoError(t, cc.Set(ctx, "k", value, time.Minute))
	require.Equal(t, 1.0, opsCount(t, m, "redis", "set", "ok"))

	require.NoError(t, cc.Get(ctx, "k", &result))
	require.Equal(t, value, result, "decorator must not alter the decoded value")
	require.Equal(t, 1.0, opsCount(t, m, "redis", "get", "hit"))
	require.Equal(t, 1.0, opsCount(t, m, "redis", "get", "miss"))
	require.Equal(t, uint64(2), durationCount(t, m, "redis", "get"))

	t.Run("decode failure is an error, not a miss", func(t *testing.T) {
		var wrong int

		err := cc.Get(ctx, "k", &wrong)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrCacheMiss)
		require.Equal(t, 1.0, opsCount(t, m, "redis", "get", "error"))
	})
}

func TestInstrumentedErrors(t *testing.T) {
	m := NewMetrics(prometheus.NewPedanticRegistry())
	cc := NewInstrumented(failingCache{}, "fake", m)
	ctx := t.Context()

	var result string

	require.ErrorIs(t, cc.Get(ctx, "k", &result), errFakeCache)
	require.ErrorIs(t, cc.Set(ctx, "k", "v", time.Minute), errFakeCache)
	require.ErrorIs(t, cc.Delete(ctx, "k"), errFakeCache)
	require.ErrorIs(t, cc.Flush(ctx), errFakeCache)

	for _, op := range []string{"get", "set", "delete", "flush"} {
		require.Equal(t, 1.0, opsCount(t, m, "fake", op, "error"), op)
		require.Equal(t, uint64(1), durationCount(t, m, "fake", op), op)
	}

	require.Equal(t, 0.0, opsCount(t, m, "fake", "get", "miss"))
	require.Equal(t, 0.0, opsCount(t, m, "fake", "get", "hit"))
	require.Equal(t, 0.0, opsCount(t, m, "fake", "set", "ok"))
}
