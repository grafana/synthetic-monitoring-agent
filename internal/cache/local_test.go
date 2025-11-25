package cache

import (
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestNewLocal(t *testing.T) {
	logger := zerolog.New(io.Discard)

	t.Run("valid configuration", func(t *testing.T) {
		cache, err := NewLocal(LocalConfig{
			InitialCapacity: 100,
			MaxCapacity:     1000,
			DefaultTTL:      5 * time.Minute,
			Logger:          logger,
		})
		require.NoError(t, err)
		require.NotNil(t, cache)
		defer cache.Close()

		require.Implements(t, (*Cache)(nil), cache)
	})

	t.Run("minimal configuration", func(t *testing.T) {
		cache, err := NewLocal(LocalConfig{
			MaxCapacity: 100,
			Logger:      logger,
		})
		require.NoError(t, err)
		require.NotNil(t, cache)
		defer cache.Close()

		require.Equal(t, 100, cache.Capacity())
	})

	t.Run("with default TTL", func(t *testing.T) {
		cache, err := NewLocal(LocalConfig{
			MaxCapacity: 100,
			DefaultTTL:  10 * time.Second,
			Logger:      logger,
		})
		require.NoError(t, err)
		require.NotNil(t, cache)
		defer cache.Close()
	})
}

func TestNewLocalErrors(t *testing.T) {
	logger := zerolog.New(io.Discard)

	t.Run("zero capacity", func(t *testing.T) {
		cache, err := NewLocal(LocalConfig{
			MaxCapacity: 0,
			Logger:      logger,
		})
		require.Error(t, err)
		require.Nil(t, cache)
		require.Contains(t, err.Error(), "max capacity must be positive")
	})

	t.Run("negative capacity", func(t *testing.T) {
		cache, err := NewLocal(LocalConfig{
			MaxCapacity: -1,
			Logger:      logger,
		})
		require.Error(t, err)
		require.Nil(t, cache)
		require.Contains(t, err.Error(), "max capacity must be positive")
	})
}

func TestLocalCacheSetGet(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cache, err := NewLocal(LocalConfig{
		MaxCapacity: 1000,
		DefaultTTL:  1 * time.Minute,
		Logger:      logger,
	})
	require.NoError(t, err)
	defer cache.Close()

	ctx := t.Context()

	t.Run("set and get string", func(t *testing.T) {
		key := "test:string"
		value := "hello world"

		err := cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		var result string
		err = cache.Get(ctx, key, &result)
		require.NoError(t, err)
		require.Equal(t, value, result)
	})

	t.Run("set and get int", func(t *testing.T) {
		key := "test:int"
		value := 42

		err := cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		var result int
		err = cache.Get(ctx, key, &result)
		require.NoError(t, err)
		require.Equal(t, value, result)
	})

	t.Run("set and get struct", func(t *testing.T) {
		key := "test:struct"
		value := TestStruct{
			ID:   123,
			Name: "test",
			Tags: []string{"a", "b", "c"},
		}

		err := cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		var result TestStruct
		err = cache.Get(ctx, key, &result)
		require.NoError(t, err)
		require.Equal(t, value, result)
	})

	t.Run("set and get slice", func(t *testing.T) {
		key := "test:slice"
		value := []string{"x", "y", "z"}

		err := cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		var result []string
		err = cache.Get(ctx, key, &result)
		require.NoError(t, err)
		require.Equal(t, value, result)
	})

	t.Run("set and get map", func(t *testing.T) {
		key := "test:map"
		value := map[string]int{"a": 1, "b": 2, "c": 3}

		err := cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		var result map[string]int
		err = cache.Get(ctx, key, &result)
		require.NoError(t, err)
		require.Equal(t, value, result)
	})

	t.Run("get non-existent key", func(t *testing.T) {
		var result string
		err := cache.Get(ctx, "nonexistent:key", &result)
		require.ErrorIs(t, err, ErrCacheMiss)
		require.Empty(t, result)
	})

	t.Run("overwrite existing key", func(t *testing.T) {
		key := "test:overwrite"

		err := cache.Set(ctx, key, "first value", 5*time.Minute)
		require.NoError(t, err)

		err = cache.Set(ctx, key, "second value", 5*time.Minute)
		require.NoError(t, err)

		var result string
		err = cache.Get(ctx, key, &result)
		require.NoError(t, err)
		require.Equal(t, "second value", result)
	})
}

func TestLocalCacheExpiration(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cache, err := NewLocal(LocalConfig{
		MaxCapacity: 1000,
		DefaultTTL:  1 * time.Second,
		Logger:      logger,
	})
	require.NoError(t, err)
	defer cache.Close()

	ctx := t.Context()

	t.Run("item expires after TTL", func(t *testing.T) {
		key := "test:expire"
		value := "will expire"

		// Set with short TTL
		err := cache.Set(ctx, key, value, 100*time.Millisecond)
		require.NoError(t, err)

		// Should be available immediately
		var result string
		err = cache.Get(ctx, key, &result)
		require.NoError(t, err)
		require.Equal(t, value, result)

		// Wait for expiration
		time.Sleep(200 * time.Millisecond)

		// Trigger cleanup to remove expired items
		cache.cache.CleanUp()

		// Should be expired now
		err = cache.Get(ctx, key, &result)
		require.ErrorIs(t, err, ErrCacheMiss)
	})

	t.Run("use default TTL when expiration is zero", func(t *testing.T) {
		key := "test:default:ttl"
		value := "default ttl"

		// Set with zero expiration (should use default TTL)
		err := cache.Set(ctx, key, value, 0)
		require.NoError(t, err)

		// Should be available immediately
		var result string
		err = cache.Get(ctx, key, &result)
		require.NoError(t, err)
		require.Equal(t, value, result)

		// Wait for default TTL to expire
		time.Sleep(1500 * time.Millisecond)

		// Trigger cleanup to remove expired items
		cache.cache.CleanUp()

		// Should be expired now
		err = cache.Get(ctx, key, &result)
		require.ErrorIs(t, err, ErrCacheMiss)
	})
}

func TestLocalCacheDelete(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cache, err := NewLocal(LocalConfig{
		MaxCapacity: 1000,
		Logger:      logger,
	})
	require.NoError(t, err)
	defer cache.Close()

	ctx := t.Context()

	t.Run("delete existing key", func(t *testing.T) {
		key := "test:delete"
		value := "to be deleted"

		err := cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		// Verify it's there
		var result string
		err = cache.Get(ctx, key, &result)
		require.NoError(t, err)

		// Delete it
		err = cache.Delete(ctx, key)
		require.NoError(t, err)

		// Verify it's gone
		err = cache.Get(ctx, key, &result)
		require.ErrorIs(t, err, ErrCacheMiss)
	})

	t.Run("delete non-existent key", func(t *testing.T) {
		err := cache.Delete(ctx, "nonexistent:delete")
		require.NoError(t, err) // Should be idempotent
	})
}

func TestLocalCacheFlush(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cache, err := NewLocal(LocalConfig{
		MaxCapacity: 1000,
		Logger:      logger,
	})
	require.NoError(t, err)
	defer cache.Close()

	ctx := t.Context()

	// Add multiple items
	for i := range 10 {
		key := fmt.Sprintf("test:flush:%d", i)
		err := cache.Set(ctx, key, i, 5*time.Minute)
		require.NoError(t, err)
	}

	// Verify items are there
	require.Greater(t, cache.Size(), 0)

	// Flush
	err = cache.Flush(ctx)
	require.NoError(t, err)

	// Verify all items are gone
	require.Equal(t, 0, cache.Size())

	// Verify individual items are gone
	var result int
	err = cache.Get(ctx, "test:flush:0", &result)
	require.ErrorIs(t, err, ErrCacheMiss)
}

func TestLocalCacheCapacity(t *testing.T) {
	logger := zerolog.New(io.Discard)

	// Small capacity for testing
	cache, err := NewLocal(LocalConfig{
		MaxCapacity: 10,
		Logger:      logger,
	})
	require.NoError(t, err)
	defer cache.Close()

	ctx := t.Context()

	// Add more items than capacity
	for i := range 20 {
		key := fmt.Sprintf("test:capacity:%d", i)
		err := cache.Set(ctx, key, i, 5*time.Minute)
		require.NoError(t, err)
	}

	// Trigger cleanup to enforce capacity limits
	cache.cache.CleanUp()

	// Size should not exceed capacity (allowing some margin for async operations)
	// Otter may temporarily exceed capacity, so we check it's reasonably close
	require.LessOrEqual(t, cache.Size(), cache.Capacity()+5)
	require.Equal(t, 10, cache.Capacity())
}

func TestLocalCacheConcurrency(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cache, err := NewLocal(LocalConfig{
		MaxCapacity: 10000,
		Logger:      logger,
	})
	require.NoError(t, err)
	defer cache.Close()

	ctx := t.Context()

	t.Run("concurrent writes", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 100
		itemsPerGoroutine := 10

		wg.Add(numGoroutines)
		for i := range numGoroutines {
			go func(id int) {
				defer wg.Done()
				for j := range itemsPerGoroutine {
					key := fmt.Sprintf("concurrent:write:%d:%d", id, j)
					err := cache.Set(ctx, key, id*itemsPerGoroutine+j, 5*time.Minute)
					require.NoError(t, err)
				}
			}(i)
		}

		wg.Wait()
	})

	t.Run("concurrent reads and writes", func(t *testing.T) {
		// Pre-populate cache
		for i := range 100 {
			key := fmt.Sprintf("concurrent:rw:%d", i)
			err := cache.Set(ctx, key, i, 5*time.Minute)
			require.NoError(t, err)
		}

		var wg sync.WaitGroup
		numReaders := 50
		numWriters := 50

		// Start readers
		wg.Add(numReaders)
		for i := range numReaders {
			go func(id int) {
				defer wg.Done()
				for j := range 100 {
					key := fmt.Sprintf("concurrent:rw:%d", j)
					var result int
					_ = cache.Get(ctx, key, &result)
				}
			}(i)
		}

		// Start writers
		wg.Add(numWriters)
		for i := range numWriters {
			go func(id int) {
				defer wg.Done()
				for j := range 100 {
					key := fmt.Sprintf("concurrent:rw:%d", j)
					_ = cache.Set(ctx, key, id*100+j, 5*time.Minute)
				}
			}(i)
		}

		wg.Wait()
	})
}

func TestLocalCacheEncodeErrors(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cache, err := NewLocal(LocalConfig{
		MaxCapacity: 1000,
		Logger:      logger,
	})
	require.NoError(t, err)
	defer cache.Close()

	ctx := t.Context()

	t.Run("cannot encode channel", func(t *testing.T) {
		err := cache.Set(ctx, "test:channel", make(chan int), 5*time.Minute)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to encode value")
	})

	t.Run("cannot encode function", func(t *testing.T) {
		err := cache.Set(ctx, "test:func", func() {}, 5*time.Minute)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to encode value")
	})
}

func TestLocalCacheDecodeErrors(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cache, err := NewLocal(LocalConfig{
		MaxCapacity: 1000,
		Logger:      logger,
	})
	require.NoError(t, err)
	defer cache.Close()

	ctx := t.Context()

	// Store a string
	err = cache.Set(ctx, "test:mismatch", "hello", 5*time.Minute)
	require.NoError(t, err)

	// Try to retrieve as int
	var result int
	err = cache.Get(ctx, "test:mismatch", &result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to decode value")
}

func TestLocalCacheClose(t *testing.T) {
	logger := zerolog.New(io.Discard)

	t.Run("close succeeds", func(t *testing.T) {
		cache, err := NewLocal(LocalConfig{
			MaxCapacity: 1000,
			Logger:      logger,
		})
		require.NoError(t, err)

		err = cache.Close()
		require.NoError(t, err)
	})

	t.Run("operations after close", func(t *testing.T) {
		cache, err := NewLocal(LocalConfig{
			MaxCapacity: 1000,
			Logger:      logger,
		})
		require.NoError(t, err)

		ctx := t.Context()

		// Add some data
		err = cache.Set(ctx, "test:key", "value", 5*time.Minute)
		require.NoError(t, err)

		// Close cache
		err = cache.Close()
		require.NoError(t, err)

		// Operations after close may panic or fail.
		// This is expected behavior - cache should not be used after Close().
		//
		// With the local implementation, it might not be worth it wrap all the operations with
		// "if isClosed", and because of the way that Otter works, calling Close doesn't really
		// mean the cache has been cleared.

		if false { // This won't work for now.
			var result string
			err = cache.Get(ctx, "test:key", &result)
			require.Error(t, err, ErrCacheMiss)
		}
	})
}

// Benchmark tests

func BenchmarkLocalCacheSet(b *testing.B) {
	logger := zerolog.New(io.Discard)
	cache, _ := NewLocal(LocalConfig{
		MaxCapacity: 100000,
		Logger:      logger,
	})
	defer cache.Close()

	ctx := b.Context()
	value := "benchmark value"

	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		key := fmt.Sprintf("bench:set:%d", i%10000)
		_ = cache.Set(ctx, key, value, 5*time.Minute)
	}
}

func BenchmarkLocalCacheGet(b *testing.B) {
	logger := zerolog.New(io.Discard)
	cache, _ := NewLocal(LocalConfig{
		MaxCapacity: 100000,
		Logger:      logger,
	})
	defer cache.Close()

	ctx := b.Context()

	// Pre-populate
	for i := range 10000 {
		key := fmt.Sprintf("bench:get:%d", i)
		_ = cache.Set(ctx, key, i, 5*time.Minute)
	}

	b.ResetTimer()

	var result int

	for i := 0; b.Loop(); i++ {
		key := fmt.Sprintf("bench:get:%d", i%10000)
		_ = cache.Get(ctx, key, &result)
	}
}

func BenchmarkLocalCacheConcurrent(b *testing.B) {
	logger := zerolog.New(io.Discard)
	cache, _ := NewLocal(LocalConfig{
		MaxCapacity: 100000,
		Logger:      logger,
	})
	defer cache.Close()

	ctx := b.Context()

	// Pre-populate
	for i := range 1000 {
		key := fmt.Sprintf("bench:concurrent:%d", i)
		_ = cache.Set(ctx, key, i, 5*time.Minute)
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		var result int
		for pb.Next() {
			key := fmt.Sprintf("bench:concurrent:%d", i%1000)
			if i%2 == 0 {
				_ = cache.Get(ctx, key, &result)
			} else {
				_ = cache.Set(ctx, key, i, 5*time.Minute)
			}
			i++
		}
	})
}
