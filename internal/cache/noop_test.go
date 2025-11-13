package cache

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestNoopCache(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cache := NewNoop(logger)

	ctx := t.Context()

	t.Run("implements Cache interface", func(t *testing.T) {
		require.NotNil(t, cache)
		require.Implements(t, (*Cache)(nil), cache)
	})

	t.Run("get always returns ErrCacheMiss", func(t *testing.T) {
		var result string
		err := cache.Get(ctx, "any-key", &result)
		require.ErrorIs(t, err, ErrCacheMiss)
		require.Empty(t, result)
	})

	t.Run("set succeeds but discards value", func(t *testing.T) {
		err := cache.Set(ctx, "test-key", "test-value", 1*time.Minute)
		require.NoError(t, err)

		// Verify it wasn't actually cached
		var result string
		err = cache.Get(ctx, "test-key", &result)
		require.ErrorIs(t, err, ErrCacheMiss)
		require.Empty(t, result)
	})

	t.Run("delete succeeds", func(t *testing.T) {
		err := cache.Delete(ctx, "any-key")
		require.NoError(t, err)
	})

	t.Run("flush succeeds", func(t *testing.T) {
		err := cache.Flush(ctx)
		require.NoError(t, err)
	})
}

func TestNoopCacheSetGetDelete(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cache := NewNoop(logger)
	ctx := context.Background()

	// Set multiple values
	require.NoError(t, cache.Set(ctx, "key1", "value1", 1*time.Minute))
	require.NoError(t, cache.Set(ctx, "key2", 123, 1*time.Minute))
	require.NoError(t, cache.Set(ctx, "key3", TestStruct{ID: 1, Name: "test"}, 1*time.Minute))

	// All gets should return ErrCacheMiss
	var s string
	require.ErrorIs(t, cache.Get(ctx, "key1", &s), ErrCacheMiss)
	require.Empty(t, s)

	var i int
	require.ErrorIs(t, cache.Get(ctx, "key2", &i), ErrCacheMiss)
	require.Zero(t, i)

	var ts TestStruct
	require.ErrorIs(t, cache.Get(ctx, "key3", &ts), ErrCacheMiss)
	require.Zero(t, ts)

	// Delete should succeed
	require.NoError(t, cache.Delete(ctx, "key1"))

	// Flush should succeed
	require.NoError(t, cache.Flush(ctx))
}

func TestNoopCacheWithVariousTypes(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cache := NewNoop(logger)
	ctx := context.Background()

	testCases := map[string]struct {
		key   string
		value any
	}{
		"string": {
			key:   "test:string",
			value: "hello",
		},
		"int": {
			key:   "test:int",
			value: 42,
		},
		"struct": {
			key: "test:struct",
			value: TestStruct{
				ID:   100,
				Name: "noop test",
				Tags: []string{"a", "b"},
			},
		},
		"slice": {
			key:   "test:slice",
			value: []string{"x", "y", "z"},
		},
		"map": {
			key:   "test:map",
			value: map[string]int{"a": 1, "b": 2},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Set should succeed
			err := cache.Set(ctx, tc.key, tc.value, 5*time.Minute)
			require.NoError(t, err)

			// Get should always return ErrCacheMiss
			var result any
			err = cache.Get(ctx, tc.key, &result)
			require.ErrorIs(t, err, ErrCacheMiss)
		})
	}
}

func TestNoopCacheZeroExpiration(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cache := NewNoop(logger)
	ctx := context.Background()

	// Set with zero expiration should succeed
	err := cache.Set(ctx, "key", "value", 0)
	require.NoError(t, err)

	// But Get should still return ErrCacheMiss
	var result string
	err = cache.Get(ctx, "key", &result)
	require.ErrorIs(t, err, ErrCacheMiss)
}
