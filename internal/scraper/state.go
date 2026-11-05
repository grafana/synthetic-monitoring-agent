package scraper

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"

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
	stateKeyPrefix    = "sm:scraper:v1:"
	stateSlack        = 2 * time.Minute // aliveness slack beyond nextFire
	stateWriteTimeout = 100 * time.Millisecond
	stateReadTimeout  = 100 * time.Millisecond
	// stateLoadSpread bounds the random delay before a starting scraper reads
	// its record, so that a pod restart does not issue thousands of Gets (and,
	// with memcached, open as many connections) in the same instant. It is
	// well inside the up-to-2m jitter a cold start would have applied anyway.
	stateLoadSpread = 5 * time.Second
	// stateCatchUpJitter bounds the delay of a catch-up execution, so that a
	// pod restarting past many predecessors' NextFire does not run them all at once.
	stateCatchUpJitter = 30 * time.Second
)

const (
	stateResultOK             = "ok"
	stateResultError          = "error"
	stateResultSkippedExpired = "skipped_expired"
)

var stateResults = []string{stateResultOK, stateResultError, stateResultSkippedExpired}

const (
	stateReadHitResume  = "hit_resume"
	stateReadHitCatchUp = "hit_catch_up"
	stateReadRejected   = "rejected"
	stateReadMiss       = "miss"
	stateReadError      = "error"
	stateReadDecodeErr  = "decode_error"
)

var stateReadResults = []string{
	stateReadHitResume, stateReadHitCatchUp, stateReadRejected, stateReadMiss, stateReadError, stateReadDecodeErr,
}

func stateKey(check model.Check) string {
	return stateKeyPrefix + strconv.FormatInt(int64(check.GlobalID()), 10) + ":" + check.ConfigVersion()
}

// A single encoder and decoder serve all the scrapers. EncodeAll and DecodeAll are
// safe for concurrent use; with WithEncoderConcurrency(1) concurrent encodes
// from different scrapers are serialized through the single encoder state.
// That is accepted: a level-1 encode of a payload up to a few hundred KB takes
// well under a millisecond, and it keeps the codec's memory independent of the
// number of scrapers.
var (
	stateEncoder = newStateEncoder()
	stateDecoder = newStateDecoder()
)

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

func encodeStatePayload(ts TimeSeries) ([]byte, error) {
	data, err := (&prompb.WriteRequest{Timeseries: ts}).Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshaling write request: %w", err)
	}

	return stateEncoder.EncodeAll(data, nil), nil
}

func decodeStatePayload(b []byte) (TimeSeries, error) {
	data, err := stateDecoder.DecodeAll(b, nil)
	if err != nil {
		return nil, fmt.Errorf("decompressing write request: %w", err)
	}

	var req prompb.WriteRequest

	if err := req.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("unmarshaling write request: %w", err)
	}

	return req.Timeseries, nil
}

// StateCache persists scraper state for every scraper in the agent and hands
// a starting scraper its predecessor's record. It owns the per-call timeouts
// and the metrics reporting each outcome, so a scraper only ever calls Store
// and Load.
//
// Both are synchronous and unbounded by design. Store runs on the scraper's
// own goroutine after Publish has already handed the payload to the pusher,
// so the only thing a slow cache delays is that scraper returning to its
// ticker, by at most the write timeout. Load runs before the scraper's first
// tick, when it has nothing else to do.
// Every scraper is already a goroutine, so this costs no new ones; concurrency is
// naturally bounded by the number of scrapers finishing in the same window.
type StateCache struct {
	store       StateStore
	timeout     time.Duration
	readTimeout time.Duration
	// loadSpread and catchUpJitter are the upper bounds of the random delays
	// the scraper applies around Load; tests zero them.
	loadSpread    time.Duration
	catchUpJitter time.Duration
	metrics       stateCacheMetrics
	logger        zerolog.Logger
}

type stateCacheMetrics struct {
	writes      *prometheus.CounterVec
	recordBytes prometheus.Histogram
	reads       *prometheus.CounterVec
	recordAge   prometheus.Histogram
}

// NewStateCache returns a cache backed by store. A nil store yields a
// disabled cache: Store returns immediately, Load never finds anything and no
// metrics are registered.
func NewStateCache(store StateStore, reg prometheus.Registerer, logger zerolog.Logger) *StateCache {
	c := &StateCache{
		store:  store,
		logger: logger,
	}

	if store == nil {
		return c
	}

	c.timeout = stateWriteTimeout
	c.readTimeout = stateReadTimeout
	c.loadSpread = stateLoadSpread
	c.catchUpJitter = stateCatchUpJitter
	c.metrics = newStateCacheMetrics(reg)

	return c
}

func newStateCacheMetrics(reg prometheus.Registerer) stateCacheMetrics {
	m := stateCacheMetrics{
		writes: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "scraper_cache",
			Name:      "writes_total",
			Help:      "Total number of scraper state writes by outcome.",
		}, []string{"result"}),
		recordBytes: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
			Namespace: "sm_agent",
			Subsystem: "scraper_cache",
			Name:      "record_bytes",
			Help:      "Size of the encoded payload in each scraper state record.",
			Buckets:   prometheus.ExponentialBuckets(1024, 2, 10),
		}),
		reads: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "scraper_cache",
			Name:      "reads_total",
			Help:      "Total number of scraper state reads at scraper start by outcome.",
		}, []string{"result"}),
		recordAge: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
			Namespace: "sm_agent",
			Subsystem: "scraper_cache",
			Name:      "record_age_seconds",
			Help:      "Time since the predecessor's last execution for each scraper state record used at start.",
			Buckets:   prometheus.ExponentialBuckets(30, 2, 8),
		}),
	}

	for _, result := range stateResults {
		m.writes.WithLabelValues(result)
	}

	for _, result := range stateReadResults {
		m.reads.WithLabelValues(result)
	}

	return m
}

// Enabled reports whether Store persists anything and Load can find anything.
func (c *StateCache) Enabled() bool {
	return c.store != nil
}

// Store records the payload published for check at firedAt, waiting at most
// stateWriteTimeout for the cache. Whether a check's state is worth persisting
// at all is the scraper's decision (see Scraper.persistState); Store only
// skips records whose TTL is already non-positive.
func (c *StateCache) Store(ctx context.Context, check model.Check, firedAt time.Time, ts TimeSeries) {
	if !c.Enabled() {
		return
	}

	nextFire := firedAt.Add(ms(check.Frequency))

	// Not an error: a record that would already have expired cannot help a
	// successor, and a zero expiration would mean "keep forever" to the cache.
	ttl := time.Until(nextFire.Add(stateSlack))
	if ttl <= 0 {
		c.metrics.writes.WithLabelValues(stateResultSkippedExpired).Inc()

		return
	}

	key := stateKey(check)

	payload, err := encodeStatePayload(ts)
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

	// ctx is the scraper's context. Stop() (check deleted, updated or moved to
	// another probe) does not cancel it, so the write completes; it is only
	// cancelled when the process is shutting down, and then the record is moot.
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	if err := c.store.Set(ctx, key, record, ttl); err != nil {
		c.metrics.writes.WithLabelValues(stateResultError).Inc()
		c.logger.Warn().Err(err).Str("key", key).Msg("storing scraper state")

		return
	}

	c.metrics.writes.WithLabelValues(stateResultOK).Inc()
}

// Load returns the record a predecessor left for check, with its payload
// decoded. The context carries a stateReadTimeout deadline; a backend that
// does not watch the context (memcached) is bounded by its own client
// timeout instead. ok is false on a miss, a cache error or an undecodable
// payload, each counted under its own result; whether a found record is
// usable is the caller's decision, counted through recordRead.
func (c *StateCache) Load(ctx context.Context, check model.Check) (rec StateRecord, ts TimeSeries, ok bool) {
	if !c.Enabled() {
		return StateRecord{}, nil, false
	}

	key := stateKey(check)

	ctx, cancel := context.WithTimeout(ctx, c.readTimeout)
	defer cancel()

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

	ts, err := decodeStatePayload(rec.Payload)
	if err != nil {
		c.metrics.reads.WithLabelValues(stateReadDecodeErr).Inc()
		c.logger.Warn().Err(err).Str("key", key).Msg("decoding scraper state")

		return StateRecord{}, nil, false
	}

	return rec, ts, true
}

func (c *StateCache) recordRead(result string) {
	c.metrics.reads.WithLabelValues(result).Inc()
}

func (c *StateCache) observeRecordAge(age time.Duration) {
	c.metrics.recordAge.Observe(age.Seconds())
}
