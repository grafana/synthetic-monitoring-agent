package cache

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/rs/zerolog"
)

const (
	// DefaultTimeout is the default connection timeout for memcached servers
	DefaultTimeout = 100 * time.Millisecond

	// DefaultMaxIdleConns is the default maximum number of idle connections per server
	DefaultMaxIdleConns = 2

	// MaxKeyLength is the maximum key length supported by memcached
	MaxKeyLength = 250
)

// MemcachedConfig holds configuration for the cache client.
type MemcachedConfig struct {
	// Servers is the list of memcached server addresses (host:port)
	Servers []string
	// Logger is the logger instance for the cache client
	Logger zerolog.Logger
	// Timeout is the connection timeout for memcached operations (optional, defaults to 100ms)
	Timeout time.Duration
	// MaxIdleConns is the maximum number of idle connections per server (optional, defaults to 2)
	MaxIdleConns int
}

// MemcachedClient wraps the memcached client with additional functionality.
type MemcachedClient struct {
	mc     *memcache.Client
	logger zerolog.Logger
}

const KindMemcached Kind = "memcached"

// Ensure Client implements Cache interface at compile time
var _ Cache = (*MemcachedClient)(nil)

// NewMemcachedClient creates a new memcached cache client with the provided configuration.
// It validates the configuration and returns an error if invalid.
// Returns a Cache interface that can be used for all cache operations.
func NewMemcachedClient(config MemcachedConfig) (*MemcachedClient, error) {
	// Validate that at least one server is provided
	if len(config.Servers) == 0 {
		return nil, fmt.Errorf("at least one memcached server must be provided")
	}

	// Validate server addresses
	for i, server := range config.Servers {
		server = strings.TrimSpace(server)
		if server == "" {
			return nil, fmt.Errorf("server address at index %d is empty", i)
		}
		// Validate host:port format
		if _, _, err := net.SplitHostPort(server); err != nil {
			return nil, fmt.Errorf("invalid server address %q: %w", server, err)
		}
		config.Servers[i] = server
	}

	// Apply defaults
	if config.Timeout == 0 {
		config.Timeout = DefaultTimeout
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = DefaultMaxIdleConns
	}

	// Create memcache client
	mc := memcache.New(config.Servers...)
	mc.Timeout = config.Timeout
	mc.MaxIdleConns = config.MaxIdleConns

	client := &MemcachedClient{
		mc:     mc,
		logger: config.Logger.With().Str("component", "cache").Logger(),
	}

	client.logger.Info().
		Strs("servers", config.Servers).
		Dur("timeout", config.Timeout).
		Int("max_idle_conns", config.MaxIdleConns).
		Msg("cache client initialized")

	return client, nil
}

// Set stores a value in the cache with the specified expiration time.
// The value is serialized using gob encoding before storage.
//
// Key must be non-empty and at most 250 bytes.
// Expiration is converted to seconds (memcached requires int32 seconds).
// Use expiration of 0 for no expiration (cache default).
//
// Returns an error if:
//   - key is invalid (empty or too long)
//   - value cannot be serialized
//   - memcached operation fails
func (c *MemcachedClient) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	// Validate key
	if err := validateKey(key); err != nil {
		return err
	}

	// Serialize value
	data, err := encode(value)
	if err != nil {
		c.logger.Error().Err(err).Str("key", key).Msg("failed to encode value")
		return fmt.Errorf("cache set: %w", err)
	}

	// Convert expiration to seconds (memcached uses int32 seconds)
	expirationSec := int32(expiration.Seconds())

	// Store in memcached
	item := &memcache.Item{
		Key:        key,
		Value:      data,
		Expiration: expirationSec,
	}

	if err := c.mc.Set(item); err != nil {
		c.logger.Error().Err(err).Str("key", key).Msg("cache set failed")
		return fmt.Errorf("cache set: %w", err)
	}

	c.logger.Debug().
		Str("key", key).
		Int("size", len(data)).
		Dur("expiration", expiration).
		Msg("cache set")

	return nil
}

// Get retrieves a value from the cache and deserializes it into dest.
// The dest parameter must be a pointer to the target type.
//
// Returns ErrCacheMiss if the key is not found in the cache.
// Returns an error if:
//   - key is invalid (empty or too long)
//   - dest is not a pointer
//   - value cannot be deserialized
//   - memcached operation fails (other than cache miss)
func (c *MemcachedClient) Get(ctx context.Context, key string, dest any) error {
	// Validate key
	if err := validateKey(key); err != nil {
		return err
	}

	// Get from memcached
	item, err := c.mc.Get(key)
	if err == memcache.ErrCacheMiss {
		c.logger.Debug().Str("key", key).Msg("cache miss")
		return ErrCacheMiss
	}
	if err != nil {
		c.logger.Error().Err(err).Str("key", key).Msg("cache get failed")
		return fmt.Errorf("cache get: %w", err)
	}

	// Deserialize value
	if err := decode(item.Value, dest); err != nil {
		c.logger.Error().Err(err).Str("key", key).Msg("failed to decode value")
		return fmt.Errorf("cache get: %w", err)
	}

	c.logger.Debug().
		Str("key", key).
		Int("size", len(item.Value)).
		Msg("cache hit")

	return nil
}

// Delete removes a key from the cache.
//
// Returns nil if the key doesn't exist (not an error).
// Returns an error if:
//   - key is invalid (empty or too long)
//   - memcached operation fails (other than cache miss)
func (c *MemcachedClient) Delete(ctx context.Context, key string) error {
	// Validate key
	if err := validateKey(key); err != nil {
		return err
	}

	// Delete from memcached
	if err := c.mc.Delete(key); err == memcache.ErrCacheMiss {
		// Key doesn't exist, not an error
		c.logger.Debug().Str("key", key).Msg("cache delete - key not found")
		return nil
	} else if err != nil {
		c.logger.Error().Err(err).Str("key", key).Msg("cache delete failed")
		return fmt.Errorf("cache delete: %w", err)
	}

	c.logger.Debug().Str("key", key).Msg("cache delete")

	return nil
}

// Flush removes all items from the cache.
// Use this operation cautiously as it affects all cache users.
func (c *MemcachedClient) Flush(ctx context.Context) error {
	if err := c.mc.FlushAll(); err != nil {
		c.logger.Error().Err(err).Msg("cache flush failed")
		return fmt.Errorf("cache flush: %w", err)
	}

	c.logger.Info().Msg("cache flushed")

	return nil
}

// validateKey validates a memcached key.
func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	if len(key) > MaxKeyLength {
		return fmt.Errorf("key length %d exceeds maximum %d", len(key), MaxKeyLength)
	}
	return nil
}
