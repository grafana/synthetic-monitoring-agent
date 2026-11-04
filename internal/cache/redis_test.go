package cache

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func newTestRedis(t *testing.T) (*miniredis.Miniredis, *RedisClient) {
	t.Helper()

	s := miniredis.RunT(t)

	client, err := NewRedisClient(t.Context(), RedisConfig{
		Addrs:  []string{s.Addr()},
		Logger: testLogger(),
	})
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, client.Close()) })

	return s, client
}

func TestRedisServerGone(t *testing.T) {
	s, client := newTestRedis(t)
	ctx := t.Context()

	require.NoError(t, client.Set(ctx, "k", "v", time.Minute))

	s.Close()

	assertOpError := func(t *testing.T, err error, op string) {
		t.Helper()

		require.Error(t, err)
		require.NotErrorIs(t, err, ErrCacheMiss)
		require.True(t, strings.HasPrefix(err.Error(), "cache "+op+":"), err.Error())
	}

	assertOpError(t, client.Set(ctx, "k", "v", time.Minute), "set")
	assertOpError(t, client.Delete(ctx, "k"), "delete")

	var result string

	assertOpError(t, client.Get(ctx, "k", &result), "get")
	require.Empty(t, result)

	t.Run("instrumented counts the failure", func(t *testing.T) {
		m := NewMetrics(prometheus.NewPedanticRegistry())
		cc := NewInstrumented(client, "redis", m)

		err := cc.Get(ctx, "k", &result)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrCacheMiss)
		require.Equal(t, 1.0, testutil.ToFloat64(m.ops.WithLabelValues("redis", "get", "error")))
		require.Equal(t, 0.0, testutil.ToFloat64(m.ops.WithLabelValues("redis", "get", "miss")))
		require.Equal(t, 0.0, testutil.ToFloat64(m.ops.WithLabelValues("redis", "get", "hit")))
	})
}

func TestRedisConcurrent(t *testing.T) {
	_, client := newTestRedis(t)
	ctx := t.Context()

	const (
		goroutines = 50
		iterations = 20
		sharedKeys = 5
	)

	errs := make(chan error, goroutines*iterations*4)

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for g := range goroutines {
		go func() {
			defer wg.Done()

			for j := range iterations {
				own := fmt.Sprintf("own:%d:%d", g, j)
				shared := fmt.Sprintf("shared:%d", j%sharedKeys)
				value := g*iterations + j

				if err := client.Set(ctx, own, value, time.Minute); err != nil {
					errs <- fmt.Errorf("set %s: %w", own, err)
				}

				if err := client.Set(ctx, shared, value, time.Minute); err != nil {
					errs <- fmt.Errorf("set %s: %w", shared, err)
				}

				var got int

				switch err := client.Get(ctx, own, &got); {
				case err != nil:
					errs <- fmt.Errorf("get %s: %w", own, err)
				case got != value:
					errs <- fmt.Errorf("get %s: got %d, want %d", own, got, value)
				}

				if err := client.Get(ctx, shared, &got); err != nil && !errors.Is(err, ErrCacheMiss) {
					errs <- fmt.Errorf("get %s: %w", shared, err)
				}
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
}

func TestRedisSetGet(t *testing.T) {
	s, client := newTestRedis(t)
	ctx := t.Context()

	t.Run("round-trips a struct", func(t *testing.T) {
		key := "test:struct"
		value := TestStruct{ID: 123, Name: "test", Tags: []string{"a", "b", "c"}}

		require.NoError(t, client.Set(ctx, key, value, time.Minute))

		var result TestStruct

		require.NoError(t, client.Get(ctx, key, &result))
		require.Equal(t, value, result)
	})

	t.Run("set with ttl expires", func(t *testing.T) {
		key := "test:ttl"

		require.NoError(t, client.Set(ctx, key, "v", 10*time.Second))
		require.InDelta(t, 10*time.Second, s.TTL(key), float64(time.Second))

		s.FastForward(11 * time.Second)

		var result string

		require.ErrorIs(t, client.Get(ctx, key, &result), ErrCacheMiss)
	})

	t.Run("set with zero ttl does not expire", func(t *testing.T) {
		key := "test:nottl"

		require.NoError(t, client.Set(ctx, key, "v", 0))
		require.Equal(t, time.Duration(0), s.TTL(key))
	})

	t.Run("overwrite existing key", func(t *testing.T) {
		key := "test:overwrite"

		require.NoError(t, client.Set(ctx, key, "first", time.Minute))
		require.NoError(t, client.Set(ctx, key, "second", time.Minute))

		var result string

		require.NoError(t, client.Get(ctx, key, &result))
		require.Equal(t, "second", result)
	})

	t.Run("decode mismatch is not a miss", func(t *testing.T) {
		key := "test:mismatch"

		require.NoError(t, client.Set(ctx, key, "a string", time.Minute))

		var result int

		err := client.Get(ctx, key, &result)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrCacheMiss)
		require.Contains(t, err.Error(), "failed to decode value")
	})

	t.Run("un-encodable value", func(t *testing.T) {
		err := client.Set(ctx, "test:chan", make(chan int), time.Minute)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to encode value")
	})

	t.Run("empty key", func(t *testing.T) {
		require.Error(t, client.Set(ctx, "", "v", time.Minute))

		var result string

		err := client.Get(ctx, "", &result)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrCacheMiss)
		require.Error(t, client.Delete(ctx, ""))
	})
}

func TestRedisGetMiss(t *testing.T) {
	_, client := newTestRedis(t)

	var result string

	err := client.Get(t.Context(), "nonexistent", &result)
	require.ErrorIs(t, err, ErrCacheMiss)
	require.Empty(t, result)
}

func TestRedisDeleteIdempotent(t *testing.T) {
	_, client := newTestRedis(t)
	ctx := t.Context()
	key := "test:delete"

	require.NoError(t, client.Set(ctx, key, "v", time.Minute))
	require.NoError(t, client.Delete(ctx, key))

	var result string

	require.ErrorIs(t, client.Get(ctx, key, &result), ErrCacheMiss)

	require.NoError(t, client.Delete(ctx, key))
	require.NoError(t, client.Delete(ctx, "never:existed"))
}

func TestRedisFlush(t *testing.T) {
	s, client := newTestRedis(t)
	ctx := t.Context()

	for _, key := range []string{"a", "b", "c"} {
		require.NoError(t, client.Set(ctx, key, key, time.Minute))
	}

	require.Len(t, s.Keys(), 3)

	require.NoError(t, client.Flush(ctx))
	require.Empty(t, s.Keys())

	var result string

	require.ErrorIs(t, client.Get(ctx, "a", &result), ErrCacheMiss)
}

func TestRedisContextCancelled(t *testing.T) {
	_, client := newTestRedis(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := client.Set(ctx, "k", "v", time.Minute)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrCacheMiss)

	var result string

	err = client.Get(ctx, "k", &result)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrCacheMiss)
}

func TestNewRedisClientNoAddrs(t *testing.T) {
	client, err := NewRedisClient(t.Context(), RedisConfig{Logger: testLogger()})
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "at least one redis address must be provided")
}

func TestNewRedisClientAddrs(t *testing.T) {
	t.Run("whitespace is trimmed", func(t *testing.T) {
		s := miniredis.RunT(t)
		addrs := []string{"  " + s.Addr() + "  "}

		client, err := NewRedisClient(t.Context(), RedisConfig{Addrs: addrs, Logger: testLogger()})
		require.NoError(t, err)
		require.NoError(t, client.Close())

		// The caller's slice is not modified.
		require.Equal(t, []string{"  " + s.Addr() + "  "}, addrs)
	})

	t.Run("empty entry", func(t *testing.T) {
		client, err := NewRedisClient(t.Context(), RedisConfig{
			Addrs:  []string{"redis-1:6379", "  ", "redis-2:6379"},
			Logger: testLogger(),
		})
		require.Error(t, err)
		require.Nil(t, client)
		require.Contains(t, err.Error(), "address at index 1 is empty")
	})

	t.Run("missing port", func(t *testing.T) {
		client, err := NewRedisClient(t.Context(), RedisConfig{
			Addrs:  []string{"redis-1"},
			Logger: testLogger(),
		})
		require.Error(t, err)
		require.Nil(t, client)
		require.Contains(t, err.Error(), "invalid redis address")
	})
}

func TestNewRedisClientPingFails(t *testing.T) {
	s, err := miniredis.Run()
	require.NoError(t, err)

	addr := s.Addr()
	s.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	client, err := NewRedisClient(ctx, RedisConfig{
		Addrs:  []string{addr},
		Logger: testLogger(),
	})
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "redis ping")
}

func TestRedisOptionsSentinel(t *testing.T) {
	t.Run("sentinel", func(t *testing.T) {
		opts := redisOptions(RedisConfig{
			Addrs:      []string{"sentinel-1:26379", "sentinel-2:26379"},
			MasterName: "mymaster",
			Password:   "secret",
			DB:         3,
			Timeout:    250 * time.Millisecond,
		})

		require.Equal(t, []string{"sentinel-1:26379", "sentinel-2:26379"}, opts.Addrs)
		require.Equal(t, "mymaster", opts.MasterName)
		require.Equal(t, "secret", opts.Password)
		require.Equal(t, "secret", opts.SentinelPassword)
		require.Equal(t, 3, opts.DB)
		require.Equal(t, 250*time.Millisecond, opts.DialTimeout)
		require.Equal(t, 250*time.Millisecond, opts.ReadTimeout)
		require.Equal(t, 250*time.Millisecond, opts.WriteTimeout)
		// go-redis treats 0 as "default (3)"; -1 is the only way to disable retries.
		require.Equal(t, -1, opts.MaxRetries)
		require.True(t, opts.ContextTimeoutEnabled)
	})

	t.Run("single node defaults", func(t *testing.T) {
		opts := redisOptions(RedisConfig{Addrs: []string{"redis:6379"}})

		require.Equal(t, []string{"redis:6379"}, opts.Addrs)
		require.Empty(t, opts.MasterName)
		require.Empty(t, opts.Password)
		require.Empty(t, opts.SentinelPassword)
		require.Equal(t, 0, opts.DB)
		require.Equal(t, DefaultTimeout, opts.DialTimeout)
		require.Equal(t, DefaultTimeout, opts.ReadTimeout)
		require.Equal(t, DefaultTimeout, opts.WriteTimeout)
	})
}
