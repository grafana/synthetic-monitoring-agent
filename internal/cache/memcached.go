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

	// DefaultRefreshInterval is the default interval for re-resolving server addresses
	DefaultRefreshInterval = 60 * time.Second

	// DNSResolveTimeout is the timeout for DNS resolution operations
	DNSResolveTimeout = 5 * time.Second

	// MaxKeyLength is the maximum key length supported by memcached
	MaxKeyLength = 250
)

// Resolver is an interface for DNS resolution to allow injection in tests.
type Resolver interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

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
	// RefreshInterval is the interval for re-resolving server addresses (optional, defaults to 60s)
	RefreshInterval time.Duration
	// Resolver is the DNS resolver to use (optional, defaults to net.DefaultResolver)
	// This is primarily for testing purposes.
	Resolver Resolver
}

// MemcachedClient wraps the memcached client with additional functionality.
type MemcachedClient struct {
	mc              *memcache.Client
	logger          zerolog.Logger
	serverList      *memcache.ServerList
	originalServers []string
	cancel          context.CancelFunc
	resolver        Resolver
}

const KindMemcached Kind = "memcached"

// Ensure Client implements Cache interface at compile time
var _ Cache = (*MemcachedClient)(nil)

// NewMemcachedClient creates a new memcached cache client with the provided configuration.
// It validates the configuration and returns an error if invalid.
// Returns a Cache interface that can be used for all cache operations.
func NewMemcachedClient(ctx context.Context, config MemcachedConfig) (*MemcachedClient, error) {
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

	if config.RefreshInterval == 0 {
		config.RefreshInterval = DefaultRefreshInterval
	}

	// Use default resolver if none provided
	resolver := config.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	// Create a ServerList selector for dynamic address resolution
	serverList := &memcache.ServerList{}

	// Resolve initial server addresses
	logger := config.Logger.With().Str("component", "cache").Logger()
	resolved := resolveServers(ctx, config.Servers, resolver, logger)

	// Ensure we have at least one resolved server
	if len(resolved) == 0 {
		return nil, fmt.Errorf("no servers could be resolved from the provided list: %v", config.Servers)
	}

	// Set initial servers in the selector
	if err := serverList.SetServers(resolved...); err != nil {
		return nil, fmt.Errorf("failed to set initial servers: %w", err)
	}

	// Create memcache client with the selector
	mc := memcache.NewFromSelector(serverList)
	mc.Timeout = config.Timeout
	mc.MaxIdleConns = config.MaxIdleConns

	// Create context for the refresh goroutine
	refreshCtx, cancel := context.WithCancel(ctx)

	client := &MemcachedClient{
		mc:              mc,
		logger:          logger,
		serverList:      serverList,
		originalServers: config.Servers,
		cancel:          cancel,
		resolver:        resolver,
	}

	// Start background goroutine to periodically refresh server addresses
	go client.refreshServers(refreshCtx, config.RefreshInterval)

	client.logger.Info().
		Strs("servers", config.Servers).
		Int("resolved_count", len(resolved)).
		Dur("timeout", config.Timeout).
		Int("max_idle_conns", config.MaxIdleConns).
		Dur("refresh_interval", config.RefreshInterval).
		Msg("cache client initialized with dynamic address resolution")

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

// Close stops the background server address refresh goroutine by cancelling its context.
//
// This method is optional if the parent context passed to NewMemcachedClient is already
// cancelled during application shutdown. The refresh goroutine will automatically stop
// when the parent context is cancelled.
//
// Call this method explicitly only if you need to stop the refresh goroutine before
// the application shuts down or if you're managing the client lifecycle independently.
//
// Note: This is not part of the Cache interface, so it must be called explicitly
// if cleanup is needed.
func (c *MemcachedClient) Close() error {
	if c.cancel != nil {
		c.cancel()
		c.logger.Debug().Msg("cache client closed")
	}

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

// resolveServers resolves all hostnames in the server list to their IP addresses.
// Each hostname is resolved to potentially multiple IP addresses.
// Returns a slice of "ip:port" strings ready for use with memcache.ServerList.SetServers.
// IP addresses are used as-is without resolution.
// Hostnames that fail to resolve are skipped (not included in the result).
func resolveServers(ctx context.Context, servers []string, resolver Resolver, logger zerolog.Logger) []string {
	var resolved []string

	for _, server := range servers {
		host, port, err := net.SplitHostPort(server)
		if err != nil {
			logger.Warn().Err(err).Str("server", server).Msg("failed to parse server address, skipping")
			continue
		}

		// Check if host is already an IP address
		if ip := net.ParseIP(host); ip != nil {
			// Already an IP address, use as-is
			resolved = append(resolved, server)
			logger.Debug().Str("server", server).Msg("using IP address directly")

			continue
		}

		// Try to resolve the hostname
		ips, err := resolver.LookupIP(ctx, "ip", host)
		if err != nil {
			logger.Warn().Err(err).Str("host", host).Msg("failed to resolve hostname, skipping server")

			continue
		}

		if len(ips) == 0 {
			logger.Warn().Str("host", host).Msg("no IPs resolved for hostname, skipping server")

			continue
		}

		// Add all resolved IPs with the port
		for _, ip := range ips {
			addr := net.JoinHostPort(ip.String(), port)
			resolved = append(resolved, addr)
		}

		logger.Debug().
			Str("host", host).
			Int("ip_count", len(ips)).
			Strs("resolved", resolved[len(resolved)-len(ips):]).
			Msg("resolved server addresses")
	}

	return resolved
}

// refreshServers periodically re-resolves server addresses and updates the ServerList.
func (c *MemcachedClient) refreshServers(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Debug().Msg("stopping server address refresh")

			return

		case <-ticker.C:
			// Create a timeout context for DNS resolution to prevent hanging
			resolveCtx, cancel := context.WithTimeout(ctx, DNSResolveTimeout)
			resolved := resolveServers(resolveCtx, c.originalServers, c.resolver, c.logger)
			cancel()

			if len(resolved) == 0 {
				c.logger.Error().
					Strs("servers", c.originalServers).
					Msg("failed to refresh server addresses")

				continue
			}

			if err := c.serverList.SetServers(resolved...); err != nil {
				c.logger.Error().
					Err(err).
					Strs("servers", resolved).
					Msg("failed to update server addresses")

				continue
			}

			c.logger.Debug().
				Int("server_count", len(resolved)).
				Strs("servers", resolved).
				Msg("refreshed server addresses")
		}
	}
}
