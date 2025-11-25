package cache

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// Noop is a cache implementation that does nothing.
// It always returns ErrCacheMiss for Get operations and silently
// discards all Set/Delete/Flush operations.
//
// This allows client code to use the cache without nil checks,
// simplifying the codebase and making it easier to test.
type Noop struct {
	logger zerolog.Logger
}

const KindNoop = Kind("noop")

// Ensure noopCache implements Cache interface at compile time
var _ Cache = (*Noop)(nil)

// NewNoop creates a new no-op cache that implements the Cache interface
// but performs no actual caching.
//
// This is useful when:
//   - No memcached servers are configured
//   - Cache initialization fails but the agent should continue
//   - Testing code that uses cache without running memcached
//
// The noop cache always returns ErrCacheMiss for Get operations and
// succeeds (but does nothing) for Set, Delete, and Flush operations.
func NewNoop(logger zerolog.Logger) *Noop {
	return &Noop{
		logger: logger.With().Str("component", "cache").Str("type", "noop").Logger(),
	}
}

// Set discards the value and returns nil (successful no-op).
func (n *Noop) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	n.logger.Debug().Str("key", key).Msg("noop cache set (discarded)")
	return nil
}

// Get always returns ErrCacheMiss since nothing is cached.
func (n *Noop) Get(ctx context.Context, key string, dest any) error {
	n.logger.Debug().Str("key", key).Msg("noop cache get (always miss)")
	return ErrCacheMiss
}

// Delete does nothing and returns nil (successful no-op).
func (n *Noop) Delete(ctx context.Context, key string) error {
	n.logger.Debug().Str("key", key).Msg("noop cache delete (discarded)")
	return nil
}

// Flush does nothing and returns nil (successful no-op).
func (n *Noop) Flush(ctx context.Context) error {
	n.logger.Debug().Msg("noop cache flush (discarded)")
	return nil
}
