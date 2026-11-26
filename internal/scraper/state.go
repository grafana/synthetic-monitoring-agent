package scraper

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
)

// StateStore is the interface for cache writing.
type StateStore interface {
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
}

// StateRecord is what a scraper leaves behind for its successor.
type StateRecord struct {
	NextFire time.Time // when the writer's next regular tick was due
	Payload  []byte    // zstd-1(prompb.WriteRequest{Timeseries}) of the last published payload
}

const (
	stateKeyPrefix = "sm:scraper:v1:"
	stateSlack     = 2 * time.Minute // aliveness slack beyond nextFire
)

const (
	stateResultOK             = "ok"
	stateResultError          = "error"
	stateResultSkippedExpired = "skipped_expired"
)

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

// StateWriter persists a scraper's state for its successor. A scraper calls
// Store after every execution it is eligible for and never asks whether
// anything is listening: an agent without a scraper cache hands out a
// NoopStateWriter, so the decision is made once, where the cache is configured.
type StateWriter interface {
	// Store records the payload published for check together with nextFire,
	// the check's next regular tick.
	Store(ctx context.Context, check model.Check, nextFire time.Time, ts TimeSeries)
}

// NoopStateWriter is the StateWriter of an agent that does not cache scraper
// state. It registers no metrics and costs nothing per call.
type NoopStateWriter struct{}

func (NoopStateWriter) Store(context.Context, model.Check, time.Time, TimeSeries) {}

// stateWriter persists scraper state in a cache shared across restarts and
// reports each outcome as a metric.
//
// Store is synchronous by design. It runs on the scraper's own goroutine after
// Publish has already handed the payload to the pusher, so the only thing a
// slow cache delays is that scraper returning to its ticker, by at most the
// cache client's own timeout (100ms per socket operation for memcached, see
// cache.DefaultTimeout). The context is passed on by convention; the memcached
// client does not watch it.
// Every scraper is already a goroutine, so this costs no new ones; concurrency is
// naturally bounded by the number of scrapers finishing in the same window.
type stateWriter struct {
	store StateStore
	// A single encoder and decoder serve all the scrapers. EncodeAll and DecodeAll are
	// safe for concurrent use; with WithEncoderConcurrency(1) concurrent encodes
	// from different scrapers are serialized through the single encoder state.
	// That is accepted: a level-1 encode of a payload up to a few hundred KB takes
	// well under a millisecond, and it keeps the codec's memory independent of the
	// number of scrapers.
	encoder *zstd.Encoder
	decoder *zstd.Decoder
	metrics stateWriterMetrics
	logger  zerolog.Logger
}

type stateWriterMetrics struct {
	writes      *prometheus.CounterVec
	recordBytes prometheus.Histogram
}

// NewStateWriter returns a writer backed by store, which must not be nil, and
// registers its metrics with reg.
func NewStateWriter(store StateStore, reg prometheus.Registerer, logger zerolog.Logger) *stateWriter {
	return &stateWriter{
		store:   store,
		encoder: newStateEncoder(),
		decoder: newStateDecoder(),
		metrics: newStateWriterMetrics(reg),
		logger:  logger,
	}
}

func (w *stateWriter) encode(ts TimeSeries) ([]byte, error) {
	data, err := (&prompb.WriteRequest{Timeseries: ts}).Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshaling write request: %w", err)
	}

	return w.encoder.EncodeAll(data, nil), nil
}

func (w *stateWriter) decode(b []byte) (TimeSeries, error) {
	data, err := w.decoder.DecodeAll(b, nil)
	if err != nil {
		return nil, fmt.Errorf("decompressing write request: %w", err)
	}

	var req prompb.WriteRequest

	if err := req.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("unmarshaling write request: %w", err)
	}

	return req.Timeseries, nil
}

func newStateWriterMetrics(reg prometheus.Registerer) stateWriterMetrics {
	m := stateWriterMetrics{
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
	}

	reg.MustRegister(m.writes, m.recordBytes)

	for _, result := range []string{stateResultOK, stateResultError, stateResultSkippedExpired} {
		m.writes.WithLabelValues(result)
	}

	return m
}

// Store records the payload published for check and the check's next regular
// tick. The write is bounded by the cache client's own timeout, not by ctx.
// Whether a check's state is worth persisting at all is the scraper's decision
// (see Scraper.persistState); Store only skips records whose TTL is already
// non-positive.
func (w *stateWriter) Store(ctx context.Context, check model.Check, nextFire time.Time, ts TimeSeries) {
	// Not an error: a record that would already have expired cannot help a
	// successor, and a zero expiration would mean "keep forever" to the cache.
	ttl := time.Until(nextFire.Add(stateSlack))
	if ttl <= 0 {
		w.metrics.writes.WithLabelValues(stateResultSkippedExpired).Inc()

		return
	}

	key := stateKey(check)

	payload, err := w.encode(ts)
	if err != nil {
		w.metrics.writes.WithLabelValues(stateResultError).Inc()
		w.logger.Warn().Err(err).Str("key", key).Msg("encoding scraper state")

		return
	}

	w.metrics.recordBytes.Observe(float64(len(payload)))

	record := StateRecord{
		NextFire: nextFire,
		Payload:  payload,
	}

	if err := w.store.Set(ctx, key, record, ttl); err != nil {
		w.metrics.writes.WithLabelValues(stateResultError).Inc()
		w.logger.Warn().Err(err).Str("key", key).Msg("storing scraper state")

		return
	}

	w.metrics.writes.WithLabelValues(stateResultOK).Inc()
}
