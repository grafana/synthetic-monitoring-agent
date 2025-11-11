package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/maypok86/otter/v2"
	"github.com/rs/zerolog"
)

// LocalConfig holds configuration for the local in-memory cache.
type LocalConfig struct {
	// InitialCapacity is the initial capacity of the cache
	InitialCapacity int
	// MaxCapacity is the maximum capacity (number of items) in the cache
	MaxCapacity int
	// DefaultTTL is the default time-to-live for items (if expiration not specified in Set)
	DefaultTTL time.Duration
	// Logger for debug logging
	Logger zerolog.Logger
}

// Local is an in-memory cache implementation using Otter.
// It provides fast, thread-safe caching with automatic eviction and TTL support.
type Local struct {
	cache      *otter.Cache[string, []byte]
	logger     zerolog.Logger
	defaultTTL time.Duration
}

const KindLocal Kind = "local"

// Ensure Local implements Cache interface at compile time
var _ Cache = (*Local)(nil)

// NewLocal creates a new in-memory cache with the specified configuration.
// It uses Otter for high-performance, thread-safe caching with automatic eviction.
//
// Returns an error if:
//   - MaxCapacity is not positive
//   - Cache initialization fails
func NewLocal(config LocalConfig) (*Local, error) {
	// Validate configuration
	if config.MaxCapacity <= 0 {
		return nil, fmt.Errorf("max capacity must be positive, got %d", config.MaxCapacity)
	}

	// Build Otter cache with configuration
	opts := &otter.Options[string, []byte]{
		MaximumSize:      config.MaxCapacity,
		InitialCapacity:  config.InitialCapacity,
		ExpiryCalculator: otter.ExpiryWriting[string, []byte](24 * time.Hour),
	}

	// Configure default expiration policy to enable per-item TTL with SetExpiresAfter
	// Use a very long default TTL that will be overridden per-item
	if config.DefaultTTL > 0 {
		opts.ExpiryCalculator = otter.ExpiryWriting[string, []byte](config.DefaultTTL)
	}

	cache, err := otter.New(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build cache: %w", err)
	}

	l := &Local{
		cache:      cache,
		logger:     config.Logger.With().Str("component", "cache").Str("type", "local").Logger(),
		defaultTTL: config.DefaultTTL,
	}

	l.logger.Info().
		Int("max_capacity", config.MaxCapacity).
		Int("initial_capacity", config.InitialCapacity).
		Dur("default_ttl", config.DefaultTTL).
		Msg("local cache initialized")

	return l, nil
}

// Set stores a value in the cache with the specified expiration time.
// The value is serialized using gob encoding before storage.
//
// If expiration is 0, the default TTL from LocalConfig is used.
func (l *Local) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	// Serialize value using existing encode function. In principle this is not necessary with Otter, but we need to
	// support the common interface that uses `any` values.
	data, err := encode(value)
	if err != nil {
		l.logger.Error().Err(err).Str("key", key).Msg("failed to encode value")
		return fmt.Errorf("cache set: %w", err)
	}

	// Use provided expiration or default TTL
	ttl := expiration
	if ttl == 0 {
		ttl = l.defaultTTL
	}

	// Set the value
	l.cache.Set(key, data)

	// Set expiration if TTL is specified
	if ttl > 0 {
		l.cache.SetExpiresAfter(key, ttl)
	}

	l.logger.Debug().Str("key", key).Dur("ttl", ttl).Int("size", len(data)).Msg("local cache set")

	return nil
}

// Get retrieves a value from the cache and deserializes it into dest.
// Returns ErrCacheMiss if the key is not found or has expired.
func (l *Local) Get(ctx context.Context, key string, dest any) error {
	data, ok := l.cache.GetIfPresent(key)
	if !ok {
		l.logger.Debug().Str("key", key).Msg("local cache miss")

		return ErrCacheMiss
	}

	if err := decode(data, dest); err != nil {
		l.logger.Error().Err(err).Str("key", key).Msg("failed to decode value")

		return fmt.Errorf("cache get: %w", err)
	}

	l.logger.Debug().Str("key", key).Int("size", len(data)).Msg("local cache hit")

	return nil
}

// Delete removes a key from the cache.
//
// Returns nil if the key doesn't exist (idempotent).
func (l *Local) Delete(ctx context.Context, key string) error {
	l.cache.Invalidate(key)
	l.logger.Debug().Str("key", key).Msg("local cache delete")

	return nil
}

// Flush removes all items from the cache.
func (l *Local) Flush(ctx context.Context) error {
	l.cache.InvalidateAll()
	l.logger.Info().Msg("local cache flushed")

	return nil
}

// Close cleanly shuts down the cache.
//
// Otter performs cleanup operations to release resources.
func (l *Local) Close() error {
	l.cache.CleanUp()
	l.logger.Info().Msg("local cache closed")

	return nil
}

// Size returns the current number of items in the cache.
//
// This is useful for monitoring and debugging.
func (l *Local) Size() int {
	return l.cache.EstimatedSize()
}

// Capacity returns the maximum capacity of the cache.
func (l *Local) Capacity() int {
	return int(l.cache.GetMaximum())
}
