package cache

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func TestNew(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		cache, err := NewMemcachedClient(MemcachedConfig{
			Servers: []string{"localhost:11211"},
			Logger:  testLogger(),
		})
		require.NoError(t, err)
		require.NotNil(t, cache)
		require.Implements(t, (*Cache)(nil), cache)
	})

	t.Run("multiple servers", func(t *testing.T) {
		client, err := NewMemcachedClient(MemcachedConfig{
			Servers: []string{"localhost:11211", "cache1:11211", "cache2:11211"},
			Logger:  testLogger(),
		})
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("custom timeout and max idle conns", func(t *testing.T) {
		cache, err := NewMemcachedClient(MemcachedConfig{
			Servers:      []string{"localhost:11211"},
			Logger:       testLogger(),
			Timeout:      200 * time.Millisecond,
			MaxIdleConns: 5,
		})
		require.NoError(t, err)
		require.NotNil(t, cache)

		require.Equal(t, 200*time.Millisecond, cache.mc.Timeout)
		require.Equal(t, 5, cache.mc.MaxIdleConns)
	})

	t.Run("default timeout and max idle conns", func(t *testing.T) {
		cache, err := NewMemcachedClient(MemcachedConfig{
			Servers: []string{"localhost:11211"},
			Logger:  testLogger(),
		})
		require.NoError(t, err)

		require.Equal(t, DefaultTimeout, cache.mc.Timeout)
		require.Equal(t, DefaultMaxIdleConns, cache.mc.MaxIdleConns)
	})

	t.Run("servers with whitespace", func(t *testing.T) {
		client, err := NewMemcachedClient(MemcachedConfig{
			Servers: []string{"  localhost:11211  ", " cache1:11211 "},
			Logger:  testLogger(),
		})
		require.NoError(t, err)
		require.NotNil(t, client)
	})
}

func TestNewErrors(t *testing.T) {
	t.Run("no servers", func(t *testing.T) {
		client, err := NewMemcachedClient(MemcachedConfig{
			Servers: []string{},
			Logger:  testLogger(),
		})
		require.Error(t, err)
		require.Nil(t, client)
		require.Contains(t, err.Error(), "at least one memcached server must be provided")
	})

	t.Run("empty server address", func(t *testing.T) {
		client, err := NewMemcachedClient(MemcachedConfig{
			Servers: []string{"localhost:11211", "", "cache1:11211"},
			Logger:  testLogger(),
		})
		require.Error(t, err)
		require.Nil(t, client)
		require.Contains(t, err.Error(), "server address at index 1 is empty")
	})

	t.Run("invalid server address - no port", func(t *testing.T) {
		client, err := NewMemcachedClient(MemcachedConfig{
			Servers: []string{"localhost"},
			Logger:  testLogger(),
		})
		require.Error(t, err)
		require.Nil(t, client)
		require.Contains(t, err.Error(), "invalid server address")
	})

	t.Run("invalid server address - bad format", func(t *testing.T) {
		client, err := NewMemcachedClient(MemcachedConfig{
			Servers: []string{"not a valid address"},
			Logger:  testLogger(),
		})
		require.Error(t, err)
		require.Nil(t, client)
		require.Contains(t, err.Error(), "invalid server address")
	})
}

func TestValidateKey(t *testing.T) {
	t.Run("valid key", func(t *testing.T) {
		err := validateKey("valid:key:123")
		require.NoError(t, err)
	})

	t.Run("empty key", func(t *testing.T) {
		err := validateKey("")
		require.Error(t, err)
		require.Contains(t, err.Error(), "key cannot be empty")
	})

	t.Run("key too long", func(t *testing.T) {
		longKey := string(make([]byte, MaxKeyLength+1))
		err := validateKey(longKey)
		require.Error(t, err)
		require.Contains(t, err.Error(), "key length")
		require.Contains(t, err.Error(), "exceeds maximum")
	})

	t.Run("key at max length", func(t *testing.T) {
		maxKey := string(make([]byte, MaxKeyLength))
		err := validateKey(maxKey)
		require.NoError(t, err)
	})
}

func TestSetGetDelete(t *testing.T) {
	// These tests require a running memcached instance
	// They will fail gracefully if memcached is not available
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, err := NewMemcachedClient(MemcachedConfig{
		Servers: []string{"localhost:11211"},
		Logger:  testLogger(),
	})
	require.NoError(t, err)

	err = client.mc.Ping()
	if err != nil {
		t.Skipf("memcached not available: %v", err)
	}

	ctx := context.Background()

	t.Run("set and get string", func(t *testing.T) {
		key := "test:string"
		value := "hello world"

		err := client.Set(ctx, key, value, 1*time.Minute)
		require.NoError(t, err)

		var result string
		err = client.Get(ctx, key, &result)
		require.NoError(t, err)
		require.Equal(t, value, result)

		// Clean up
		err = client.Delete(ctx, key)
		require.NoError(t, err)
	})

	t.Run("set and get struct", func(t *testing.T) {
		key := "test:struct"
		value := TestStruct{
			ID:   123,
			Name: "test",
			Tags: []string{"a", "b", "c"},
		}

		err := client.Set(ctx, key, value, 1*time.Minute)
		require.NoError(t, err)

		var result TestStruct
		err = client.Get(ctx, key, &result)
		require.NoError(t, err)
		require.Equal(t, value, result)

		// Clean up
		err = client.Delete(ctx, key)
		require.NoError(t, err)
	})

	t.Run("get non-existent key", func(t *testing.T) {
		key := "test:nonexistent"

		var result string
		err := client.Get(ctx, key, &result)
		require.ErrorIs(t, err, ErrCacheMiss)
		require.Empty(t, result)
	})

	t.Run("delete non-existent key", func(t *testing.T) {
		key := "test:nonexistent:delete"

		err := client.Delete(ctx, key)
		require.NoError(t, err) // Deleting non-existent key is not an error
	})

	t.Run("set with zero expiration", func(t *testing.T) {
		key := "test:no:expiration"
		value := "no expiration"

		err := client.Set(ctx, key, value, 0)
		require.NoError(t, err)

		var result string
		err = client.Get(ctx, key, &result)
		require.NoError(t, err)
		require.Equal(t, value, result)

		// Clean up
		err = client.Delete(ctx, key)
		require.NoError(t, err)
	})
}

func TestSetErrors(t *testing.T) {
	client, err := NewMemcachedClient(MemcachedConfig{
		Servers: []string{"localhost:11211"},
		Logger:  testLogger(),
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("empty key", func(t *testing.T) {
		err := client.Set(ctx, "", "value", 1*time.Minute)
		require.Error(t, err)
		require.Contains(t, err.Error(), "key cannot be empty")
	})

	t.Run("key too long", func(t *testing.T) {
		longKey := string(make([]byte, MaxKeyLength+1))
		err := client.Set(ctx, longKey, "value", 1*time.Minute)
		require.Error(t, err)
		require.Contains(t, err.Error(), "key length")
	})

	t.Run("un-encodable value", func(t *testing.T) {
		err := client.Set(ctx, "test:channel", make(chan int), 1*time.Minute)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to encode value")
	})
}

func TestGetErrors(t *testing.T) {
	client, err := NewMemcachedClient(MemcachedConfig{
		Servers: []string{"localhost:11211"},
		Logger:  testLogger(),
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("empty key", func(t *testing.T) {
		var result string
		err := client.Get(ctx, "", &result)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrCacheMiss)
		require.Contains(t, err.Error(), "key cannot be empty")
	})

	t.Run("key too long", func(t *testing.T) {
		longKey := string(make([]byte, MaxKeyLength+1))
		var result string
		err := client.Get(ctx, longKey, &result)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrCacheMiss)
		require.Contains(t, err.Error(), "key length")
	})
}

func TestDeleteErrors(t *testing.T) {
	client, err := NewMemcachedClient(MemcachedConfig{
		Servers: []string{"localhost:11211"},
		Logger:  testLogger(),
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("empty key", func(t *testing.T) {
		err := client.Delete(ctx, "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "key cannot be empty")
	})

	t.Run("key too long", func(t *testing.T) {
		longKey := string(make([]byte, MaxKeyLength+1))
		err := client.Delete(ctx, longKey)
		require.Error(t, err)
		require.Contains(t, err.Error(), "key length")
	})
}
