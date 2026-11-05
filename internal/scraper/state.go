package scraper

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
	"golang.org/x/sync/semaphore"

	"github.com/grafana/synthetic-monitoring-agent/internal/cache"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
)

// StateStore is the subset of cache.Cache the scraper state needs.
type StateStore interface {
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	// Get returns cache.ErrCacheMiss when key is absent.
	Get(ctx context.Context, key string, dest any) error
}

// StateRecord is what a scraper leaves behind for its successor.
type StateRecord struct {
	NextFire time.Time // when the writer's next regular tick was due
	Payload  []byte    // zstd-1(prompb.WriteRequest{Timeseries}) of the last published payload
}

const (
	stateKeyPrefix = "sm:scraper:v1:"
	stateSlack     = 2 * time.Minute // aliveness slack beyond nextFire
	// maxConcurrentReaders bounds Load calls in flight per agent. A pod restart
	// hydrates every eligible scraper at the same instant, and each call in
	// flight is a connection to the cache, so without a bound a large pod would open
	// thousands of connections against memcached's limit.
	maxConcurrentReaders = 32
	// stateCatchUpJitter applies to "catch-up" executions ([startCatchUp]): when a [StateRecord] is
	// fetched from cache, but its `nextFire` has already passed. This indicates the check
	// should have executed within the previous 2 minutes.
	// In that case, all such checks will be run immediately to "catch up" to the schedule.
	// Start times are jittered by [stateCatchUpJitter] to avoid stampeding them.
	stateCatchUpJitter = 30 * time.Second
)

const (
	stateResultOK             = "ok"
	stateResultError          = "error"
	stateResultSkippedExpired = "skipped_expired"
)

const (
	stateReadHit       = "hit"
	stateReadMiss      = "miss"
	stateReadError     = "error"
	stateReadDecodeErr = "decode_error"
)

var stateReadResults = []string{stateReadHit, stateReadMiss, stateReadError, stateReadDecodeErr}

func stateKey(check model.Check) string {
	return stateKeyPrefix + strconv.FormatInt(int64(check.GlobalID()), 10) + ":" + check.ConfigVersion()
}

func newStateEncoder() *zstd.Encoder {
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest), zstd.WithEncoderConcurrency(1))
	if err != nil {
		panic(fmt.Errorf("creating scraper state encoder: %w", err)) // only invalid options fail, and these are constants
	}

	return enc
}

func newStateDecoder() *zstd.Decoder {
	// The memory cap bounds what a corrupt or hostile record can make DecodeAll
	// allocate; real records are a few hundred KB at most. 8 MiB is plenty of
	// headroom while still keeping this bounded.
	dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(8<<20))
	if err != nil {
		panic(fmt.Errorf("creating scraper state decoder: %w", err)) // only invalid options fail, and these are constants
	}

	return dec
}

// StateCache persists a scraper's state for its successor and hands a starting
// scraper its predecessor's record. A scraper calls Store after every eligible
// execution and Load once at start, and never asks whether anything is
// listening: an agent without a scraper cache hands out a NoopStateCache, so
// the decision is made once, where the cache is configured.
type StateCache interface {
	// Store records the payload published for check together with nextFire,
	// the check's next regular tick.
	Store(ctx context.Context, check model.Check, nextFire time.Time, ts TimeSeries)
	// Load returns the record a predecessor left for check, with its payload
	// decoded; ok is false when there is none that can be used.
	Load(ctx context.Context, check model.Check) (rec StateRecord, ts TimeSeries, ok bool)
}

// NoopStateCache is the StateCache of an agent that does not cache scraper
// state. It registers no metrics, finds nothing and costs nothing per call.
type NoopStateCache struct{}

func (NoopStateCache) Store(context.Context, model.Check, time.Time, TimeSeries) {}

func (NoopStateCache) Load(context.Context, model.Check) (StateRecord, TimeSeries, bool) {
	return StateRecord{}, nil, false
}

// stateCache persists scraper state in a cache shared across restarts and
// reports each outcome as a metric.
//
// Store and Load are synchronous by design. Store runs on the scraper's own
// goroutine after Publish has already handed the payload to the pusher, so
// the only thing a slow cache delays is that scraper returning to its ticker.
// Load runs before the scraper's first tick, when it has nothing else to do.
// Both are bounded by the cache client's own timeout (100ms per socket
// operation for memcached, see cache.DefaultTimeout); the context is passed
// on by convention, the memcached client does not watch it. Every scraper is
// already a goroutine, so this costs no new ones.
type stateCache struct {
	store StateStore
	// A single encoder and decoder serve all the scrapers. EncodeAll and DecodeAll are
	// safe for concurrent use; with WithEncoderConcurrency(1) concurrent encodes
	// from different scrapers are serialized through the single encoder state.
	// That is accepted: a level-1 encode of a payload up to a few hundred KB takes
	// well under a millisecond, and it keeps the codec's memory independent of the
	// number of scrapers.
	encoder *zstd.Encoder
	decoder *zstd.Decoder
	// readerSem bounds how many callers can fetch from the cache at once;
	// agent start-up is a thundering herd of reads (see maxConcurrentReaders).
	readerSem *semaphore.Weighted
	metrics   stateCacheMetrics
	logger    zerolog.Logger
}

type stateCacheMetrics struct {
	writes      *prometheus.CounterVec
	recordBytes prometheus.Histogram
	reads       *prometheus.CounterVec
}

// NewStateCache returns a cache backed by store, which must not be nil, and
// registers its metrics with reg.
func NewStateCache(store StateStore, reg prometheus.Registerer, logger zerolog.Logger) *stateCache {
	return &stateCache{
		store:     store,
		encoder:   newStateEncoder(),
		decoder:   newStateDecoder(),
		readerSem: semaphore.NewWeighted(int64(maxConcurrentReaders)),
		metrics:   newStateCacheMetrics(reg),
		logger:    logger,
	}
}

func (c *stateCache) encode(ts TimeSeries) ([]byte, error) {
	data, err := (&prompb.WriteRequest{Timeseries: ts}).Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshaling write request: %w", err)
	}

	return c.encoder.EncodeAll(data, nil), nil
}

func (c *stateCache) decode(b []byte) (TimeSeries, error) {
	data, err := c.decoder.DecodeAll(b, nil)
	if err != nil {
		return nil, fmt.Errorf("decompressing write request: %w", err)
	}

	var req prompb.WriteRequest

	if err := req.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("unmarshaling write request: %w", err)
	}

	return req.Timeseries, nil
}

func newStateCacheMetrics(reg prometheus.Registerer) stateCacheMetrics {
	m := stateCacheMetrics{
		writes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "scraper_cache",
			Name:      "writes_total",
			Help:      "Total number of scraper state writes by outcome.",
		}, []string{"result"}),
		recordBytes: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "sm_agent",
			Subsystem: "scraper_cache",
			Name:      "record_bytes",
			Help:      "Size of the encoded payload in each scraper state record.",
			Buckets:   prometheus.ExponentialBuckets(1024, 2, 10),
		}),
		reads: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "scraper_cache",
			Name:      "reads_total",
			Help:      "Total number of scraper state reads at scraper start by outcome.",
		}, []string{"result"}),
	}

	reg.MustRegister(m.writes, m.recordBytes, m.reads)

	for _, result := range []string{stateResultOK, stateResultError, stateResultSkippedExpired} {
		m.writes.WithLabelValues(result)
	}

	for _, result := range stateReadResults {
		m.reads.WithLabelValues(result)
	}

	return m
}

// Store records the payload published for check and the check's next regular
// tick. The write is bounded by the cache client's own timeout, not by ctx.
// Whether a check's state is worth persisting at all is the scraper's decision
// (see Scraper.persistState); Store only skips records whose TTL is already
// non-positive.
func (c *stateCache) Store(ctx context.Context, check model.Check, nextFire time.Time, ts TimeSeries) {
	// Not an error: a record that would already have expired cannot help a
	// successor, and a zero expiration would mean "keep forever" to the cache.
	ttl := time.Until(nextFire.Add(stateSlack))
	if ttl <= 0 {
		c.metrics.writes.WithLabelValues(stateResultSkippedExpired).Inc()

		return
	}

	key := stateKey(check)

	payload, err := c.encode(ts)
	if err != nil {
		c.metrics.writes.WithLabelValues(stateResultError).Inc()
		c.logger.Warn().Err(err).Str("key", key).Msg("encoding scraper state")

		return
	}

	c.metrics.recordBytes.Observe(float64(len(payload)))

	record := StateRecord{
		NextFire: nextFire,
		Payload:  payload,
	}

	if err := c.store.Set(ctx, key, record, ttl); err != nil {
		c.metrics.writes.WithLabelValues(stateResultError).Inc()
		c.logger.Warn().Err(err).Str("key", key).Msg("storing scraper state")

		return
	}

	c.metrics.writes.WithLabelValues(stateResultOK).Inc()
}

// Load returns the record a predecessor left for check, with its payload
// decoded. ok is false on a miss, a cache error or an undecodable payload,
// each counted under its own result; whether a found record is usable is the
// caller's decision; the cache counts only that it was found.
//
// Load's concurrency is bounded by [stateCache.readerSem]. The wait for a
// permit honours ctx; the cache call itself is bounded by the client's own
// timeout, not by ctx.
func (c *stateCache) Load(ctx context.Context, check model.Check) (rec StateRecord, ts TimeSeries, ok bool) {
	if err := c.readerSem.Acquire(ctx, 1); err != nil {
		c.metrics.reads.WithLabelValues(stateReadError).Inc()
		c.logger.Debug().Err(err).Msg("gave up waiting to load scraper state")

		return StateRecord{}, nil, false
	}

	defer c.readerSem.Release(1)

	key := stateKey(check)

	switch err := c.store.Get(ctx, key, &rec); {
	case errors.Is(err, cache.ErrCacheMiss):
		// The normal case for a check nobody was running; not worth a log line.
		c.metrics.reads.WithLabelValues(stateReadMiss).Inc()

		return StateRecord{}, nil, false

	case err != nil:
		c.metrics.reads.WithLabelValues(stateReadError).Inc()
		c.logger.Warn().Err(err).Str("key", key).Msg("loading scraper state")

		return StateRecord{}, nil, false
	}

	ts, err := c.decode(rec.Payload)
	if err != nil {
		c.metrics.reads.WithLabelValues(stateReadDecodeErr).Inc()
		c.logger.Warn().Err(err).Str("key", key).Msg("decoding scraper state")

		return StateRecord{}, nil, false
	}

	c.metrics.reads.WithLabelValues(stateReadHit).Inc()

	return rec, ts, true
}
