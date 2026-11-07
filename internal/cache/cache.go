// Package cache provides an interface-based caching system with multiple implementations.
//
// The package defines a Cache interface that all implementations must satisfy,
// allowing the agent to use different cache backends interchangeably. This design
// eliminates the need for nil checks throughout the codebase.
//
// Available implementations:
//   - Memcached: Distributed cache using memcached servers
//   - Redis: Distributed cache using a Redis server or a Redis Sentinel deployment
//   - Local: In-process cache backed by Otter
//   - Noop: No-op cache for testing and fallback (always returns ErrCacheMiss)
//
// Any implementation can be wrapped with NewInstrumented to export operation
// counts and latencies as Prometheus metrics.
//
// The cache supports custom expiration times and automatic
// serialization/deserialization of Go values using encoding/gob.
//
// Example usage with memcached:
//
//	var cc cache.Cache
//
//	cc, err := cache.NewMemcachedClient(ctx, cache.MemcachedConfig{
//	    Servers: []string{"localhost:11211", "cache1:11211"},
//	    Logger:  logger,
//	})
//	if err != nil {
//	    // Fall back to noop cache
//	    cc = cache.NewNoop(logger)
//	}
//
//	// Store a struct
//	err = cc.Set(ctx, "user:123", user, 5*time.Minute)
//
//	// Retrieve a struct
//	var user User
//	err = cc.Get(ctx, "user:123", &user)
//	if err == cache.ErrCacheMiss {
//	    // Key not found, fetch from source
//	} else if err != nil {
//	    // Other error (network, deserialization, etc.)
//	    log.Warn("cache error: %v", err)
//	}
//
// Example usage with redis (sentinel when MasterName is set), with metrics:
//
//	rc, err := cache.NewRedisClient(ctx, cache.RedisConfig{
//	    Addrs:      []string{"sentinel-1:26379", "sentinel-2:26379"},
//	    MasterName: "mymaster",
//	    Logger:     logger,
//	})
//	cc = cache.NewInstrumented(rc, string(cache.KindRedis), cache.NewMetrics(registerer))
//
// Example usage with noop cache (for testing):
//
//	cc := cache.NewNoop(logger)
//	// All Get operations return ErrCacheMiss
//	// All Set/Delete/Flush operations succeed but do nothing
package cache

import (
	"context"
	"flag"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/error_types"
)

const (
	// ErrCacheMiss is returned when a key is not found in the cache.
	// This is a sentinel error that allows callers to distinguish between
	// a cache miss and other errors (network issues, serialization failures, etc.).
	ErrCacheMiss = error_types.BasicError("cache miss")
)

// Cache defines the interface for cache operations.
// All cache implementations (memcached, noop, local, etc.) must implement this interface.
//
// This interface allows the agent to use different cache backends interchangeably,
// including a no-op implementation that eliminates the need for nil checks in client code.
type Cache interface {
	// Set stores a value in the cache with the specified expiration time.
	// Returns an error if the operation fails.
	Set(ctx context.Context, key string, value any, expiration time.Duration) error

	// Get retrieves a value from the cache and deserializes it into dest.
	// Returns ErrCacheMiss if the key is not found.
	// Returns an error for other failures (network, serialization, etc.).
	Get(ctx context.Context, key string, dest any) error

	// Delete removes a key from the cache.
	// Returns nil if the key doesn't exist (idempotent).
	Delete(ctx context.Context, key string) error

	// Flush removes all items from the cache.
	// Use this operation cautiously as it affects all cache users.
	Flush(ctx context.Context) error
}

type Kind string

var _ flag.Value = (*Kind)(nil)

const KindAuto Kind = "auto"

const ErrUnsupportedCacheKind = error_types.BasicError("unsupported cache type")

func (val *Kind) Set(s string) error {
	switch s {
	case string(KindAuto), string(KindLocal), string(KindMemcached), string(KindNoop), string(KindRedis):
		*val = Kind(s)

		return nil

	default:
		return ErrUnsupportedCacheKind
	}
}

func (val Kind) String() string {
	return string(val)
}
