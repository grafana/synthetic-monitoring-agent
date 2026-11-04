package cache

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// RedisConfig holds configuration for the Redis cache client.
type RedisConfig struct {
	// Addrs is the list of sentinel addresses when MasterName is set, otherwise
	// the list of redis server addresses (host:port).
	Addrs []string
	// MasterName is the sentinel master set name. Empty selects a plain
	// (non-sentinel) client.
	MasterName string
	// Password is the password for the default user. It is also used to
	// authenticate against the sentinels.
	Password string
	// DB is the logical database to select.
	DB int
	// Timeout bounds dialing, reads and writes (optional, defaults to 100ms).
	Timeout time.Duration
	// Logger is the logger instance for the cache client.
	Logger zerolog.Logger
}

// RedisClient is a Cache backed by a Redis server or a Redis Sentinel deployment.
type RedisClient struct {
	rdb    redis.UniversalClient
	logger zerolog.Logger
}

const KindRedis Kind = "redis"

// Ensure RedisClient implements Cache interface at compile time
var _ Cache = (*RedisClient)(nil)

// NewRedisClient creates a new Redis cache client with the provided configuration.
// It validates the configuration, connects and issues a PING bounded by ctx, so
// an unreachable server is reported here rather than on the first operation.
func NewRedisClient(ctx context.Context, cfg RedisConfig) (*RedisClient, error) {
	if len(cfg.Addrs) == 0 {
		return nil, fmt.Errorf("at least one redis address must be provided")
	}

	addrs := make([]string, len(cfg.Addrs))

	for i, addr := range cfg.Addrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			return nil, fmt.Errorf("redis address at index %d is empty", i)
		}

		if _, _, err := net.SplitHostPort(addr); err != nil {
			return nil, fmt.Errorf("invalid redis address %q: %w", addr, err)
		}

		addrs[i] = addr
	}

	cfg.Addrs = addrs

	opts := redisOptions(cfg)
	rdb := redis.NewUniversalClient(opts)

	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()

		return nil, fmt.Errorf("redis ping: %w", err)
	}

	client := &RedisClient{
		rdb:    rdb,
		logger: cfg.Logger.With().Str("component", "cache").Str("type", "redis").Logger(),
	}

	client.logger.Info().
		Strs("addrs", cfg.Addrs).
		Str("master_name", cfg.MasterName).
		Int("db", cfg.DB).
		Dur("timeout", opts.DialTimeout).
		Msg("redis cache client initialized")

	return client, nil
}

// redisOptions maps RedisConfig onto go-redis options. A non-empty MasterName
// makes NewUniversalClient build a sentinel-backed failover client.
func redisOptions(cfg RedisConfig) *redis.UniversalOptions {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	return &redis.UniversalOptions{
		Addrs:            cfg.Addrs,
		MasterName:       cfg.MasterName,
		Password:         cfg.Password,
		SentinelPassword: cfg.Password,
		DB:               cfg.DB,
		DialTimeout:      timeout,
		ReadTimeout:      timeout,
		WriteTimeout:     timeout,
		// Callers decide whether a failed operation is worth repeating; go-redis
		// reads 0 as "default (3 retries)", so -1 is required to disable them.
		MaxRetries: -1,
		// Make the caller's context deadline bound the whole operation, not
		// just the per-step timeouts above.
		ContextTimeoutEnabled: true,
	}
}

// Set stores a value in the cache with the specified expiration time.
// The value is serialized using gob encoding before storage.
//
// An expiration <= 0 stores the key without a TTL; under a volatile-*
// eviction policy such keys are never evicted.
func (c *RedisClient) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	if err := validateRedisKey(key); err != nil {
		return err
	}

	data, err := encode(value)
	if err != nil {
		c.logger.Error().Err(err).Str("key", key).Msg("failed to encode value")
		return fmt.Errorf("cache set: %w", err)
	}

	if err := c.rdb.Set(ctx, key, data, expiration).Err(); err != nil {
		c.logger.Warn().Err(err).Str("key", key).Msg("cache set failed")
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
func (c *RedisClient) Get(ctx context.Context, key string, dest any) error {
	if err := validateRedisKey(key); err != nil {
		return err
	}

	data, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		c.logger.Debug().Str("key", key).Msg("cache miss")
		return ErrCacheMiss
	}

	if err != nil {
		c.logger.Warn().Err(err).Str("key", key).Msg("cache get failed")
		return fmt.Errorf("cache get: %w", err)
	}

	if err := decode(data, dest); err != nil {
		c.logger.Error().Err(err).Str("key", key).Msg("failed to decode value")
		return fmt.Errorf("cache get: %w", err)
	}

	c.logger.Debug().
		Str("key", key).
		Int("size", len(data)).
		Msg("cache hit")

	return nil
}

// Delete removes a key from the cache.
//
// Returns nil if the key doesn't exist (idempotent).
func (c *RedisClient) Delete(ctx context.Context, key string) error {
	if err := validateRedisKey(key); err != nil {
		return err
	}

	if err := c.rdb.Del(ctx, key).Err(); err != nil {
		c.logger.Warn().Err(err).Str("key", key).Msg("cache delete failed")
		return fmt.Errorf("cache delete: %w", err)
	}

	c.logger.Debug().Str("key", key).Msg("cache delete")

	return nil
}

// Flush removes all keys from the selected database (FLUSHDB, not FLUSHALL),
// so other databases on the same server are left alone.
func (c *RedisClient) Flush(ctx context.Context) error {
	if err := c.rdb.FlushDB(ctx).Err(); err != nil {
		c.logger.Warn().Err(err).Msg("cache flush failed")
		return fmt.Errorf("cache flush: %w", err)
	}

	c.logger.Info().Msg("cache flushed")

	return nil
}

// Close releases the underlying connections.
//
// Note: This is not part of the Cache interface, so it must be called explicitly
// if cleanup is needed.
func (c *RedisClient) Close() error {
	if err := c.rdb.Close(); err != nil {
		return fmt.Errorf("cache close: %w", err)
	}

	c.logger.Debug().Msg("cache client closed")

	return nil
}

// validateRedisKey validates a redis key. Redis has no practical key length
// limit, so only emptiness is rejected.
func validateRedisKey(key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	return nil
}
