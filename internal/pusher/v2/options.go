package v2

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
)

const (
	// This one is tricky. Most of the time, buffers are 1-2KiB in size. Are we allocating too much by default?
	defaultBufferCapacity = 4096
)

var (
	defaultPusherOptions = pusherOptions{
		// Max bytes to send on a single push request
		// Both loki and mimir can take at least 5MiB.
		maxPushBytes: 64 * 1024,

		// Max bytes to hold queued
		// 0: Disabled
		maxQueuedBytes: 128 * 1024,

		// Max items (check results) to hold in memory (per tenant per type)
		// 0: Disabled
		maxQueuedItems: 128,

		// Max time to keep an item in the queue before it's discarded
		// Note that loki/mimir will probably reject data older than 1h anyway.
		// 0: Disabled
		maxQueuedTime: time.Hour,

		// Max number of retries in case of network(retriable) error.
		// Ideally we make this big and have data expired with the above limits.
		maxRetries: 20,

		// Backoff between retries. Doubling at each attempt.
		minBackoff: time.Millisecond * 30,
		maxBackoff: time.Second * 2,

		// Max time a tenant pusher is active. This is useful to cause the tenant info
		// to be refreshed.
		maxLifetime: 2 * time.Hour,

		// Apply a jitter of 25% of maxLifetime.
		maxLifetimeJitter: 0.25,

		// How long without receiving check results until a tenant pusher is stopped.
		// This is to cleanup tenants that don't have active checks anymore.
		// Set it to a value higher than the max interval between a single check run.
		maxIdleTime: 5 * time.Minute,

		// How long to wait before refreshing tenant due to an error.
		tenantDelay: 10 * time.Second,

		// How long to stop pushing new metrics when a 429 (Too Many Requests) is received.
		waitPeriod: time.Minute,

		// How long to discard metrics when a fatal error is encountered.
		discardPeriod: 15 * time.Minute,

		pool: bufferPool{
			inner: &sync.Pool{
				New: func() interface{} {
					buf := make([]byte, 0, defaultBufferCapacity)
					return &buf
				},
			},
		},
	}
)

type pusherOptions struct {
	maxPushBytes      uint64        // Max bytes to send on a single push request
	maxQueuedBytes    uint64        // Max bytes to hold queued
	maxQueuedItems    int           // Max items (check results) to hold in memory
	maxQueuedTime     time.Duration // Max time an item can be queued until it expires
	maxRetries        int           // Max retries for a push
	minBackoff        time.Duration
	maxBackoff        time.Duration
	maxLifetime       time.Duration // How long to run a tenant pusher before re-fetching the tenant
	maxLifetimeJitter float64       // Jitter to apply to maxLifetime, expressed as % of maxLifetime
	maxIdleTime       time.Duration // How long without receiving pushes until a tenant pusher is cleaned up
	tenantDelay       time.Duration // How long to wait between GetTenant calls.
	waitPeriod        time.Duration // How long to wait in case of errors
	discardPeriod     time.Duration // How long to discard metrics when a fatal error is encountered.
	logger            zerolog.Logger
	metrics           pusher.Metrics
	pool              bufferPool
}

func (o pusherOptions) withTenant(id model.GlobalID) pusherOptions {
	localID, regionID := model.GetLocalAndRegionIDs(id)
	o.logger = o.logger.With().Int("region", regionID).Int64("tenant", localID).Logger()
	o.metrics = o.metrics.WithTenant(localID, regionID)
	return o
}

func (o pusherOptions) withType(t string) pusherOptions {
	o.logger = o.logger.With().Str("type", t).Logger()
	o.metrics = o.metrics.WithType(t)
	return o
}

func (o pusherOptions) retriesCounter() retriesCounter {
	return retriesCounter{
		left: o.maxRetries,
		max:  o.maxRetries,
	}
}

func (o pusherOptions) backOffer() backoffer {
	return backoffer{
		min: o.minBackoff,
		max: o.maxBackoff,
	}
}

type bufferPool struct {
	inner *sync.Pool
}

func (p *bufferPool) get() *[]byte {
	if p == nil || p.inner == nil {
		return nil
	}
	return p.inner.Get().(*[]byte)
}

func (p *bufferPool) put(buf *[]byte) {
	if p == nil || p.inner == nil {
		return
	}
	p.inner.Put(buf)
}

func (p *bufferPool) returnAll(records []queueEntry) (size uint64) {
	for _, rec := range records {
		size += uint64(len(*rec.data))
		p.put(rec.data)
		rec.data = nil // Ensure it can't be re-used after return.
	}
	return size
}

type retriesCounter struct {
	left, max int
}

func (c *retriesCounter) retry() bool {
	if c.left <= 0 {
		return false
	}
	c.left--
	return true
}

func (c *retriesCounter) reset() {
	c.left = c.max
}

type backoffer struct {
	last, min, max time.Duration
}

func (b *backoffer) wait(ctx context.Context) error {
	if b.last < b.min {
		b.last = b.min
	} else {
		b.last = 2 * b.last
		if b.last > b.max {
			b.last = b.max
		}
	}
	return sleepCtx(ctx, b.last)
}

func (b *backoffer) reset() {
	b.last = 0
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	select {
	case <-ctx.Done():
		t.Stop()
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
