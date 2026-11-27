package scraper

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/cache"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// stateSetCall is one Set observed by fakeStateStore, with the error it returned.
type stateSetCall struct {
	key   string
	value any
	ttl   time.Duration
	err   error
}

// stateGetCall is one Get observed by fakeStateStore, with the error it returned.
type stateGetCall struct {
	key string
	err error
}

// fakeStateStore records Set and Get calls. Set keeps the record under its key
// so a later Get finds it; records can also be seeded directly. Like the
// memcached client, it does not watch the context.
type fakeStateStore struct {
	calls   []stateSetCall
	gets    []stateGetCall
	records map[string]StateRecord
	err     error
	getErr  error
}

func (s *fakeStateStore) Set(_ context.Context, key string, value any, ttl time.Duration) error {
	s.calls = append(s.calls, stateSetCall{key: key, value: value, ttl: ttl, err: s.err})

	if rec, ok := value.(StateRecord); ok && s.err == nil {
		if s.records == nil {
			s.records = make(map[string]StateRecord)
		}

		s.records[key] = rec
	}

	return s.err
}

func (s *fakeStateStore) Get(_ context.Context, key string, dest any) error {
	err := s.getErr
	if err == nil {
		rec, found := s.records[key]
		if !found {
			err = cache.ErrCacheMiss
		} else {
			*dest.(*StateRecord) = rec
		}
	}

	s.gets = append(s.gets, stateGetCall{key: key, err: err})

	return err
}

func stateTestCheck(t *testing.T, frequency time.Duration) model.Check {
	t.Helper()

	var check model.Check

	err := check.FromSM(sm.Check{
		Id:        7,
		TenantId:  1000,
		Frequency: frequency.Milliseconds(),
		Timeout:   1000,
		Modified:  1700000000.5,
		Enabled:   true,
		Target:    "127.0.0.1",
		Job:       "test",
		Settings:  sm.CheckSettings{Ping: &sm.PingSettings{}},
	})
	require.NoError(t, err)

	return check
}

func stateTestSeries() TimeSeries {
	return TimeSeries{
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "probe_success"},
				{Name: "instance", Value: "127.0.0.1"},
				{Name: "job", Value: "test"},
			},
			Samples: []prompb.Sample{{Value: 1, Timestamp: 1700000000000}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "probe_duration_seconds"},
				{Name: "instance", Value: "127.0.0.1"},
				{Name: "job", Value: "test"},
			},
			Samples: []prompb.Sample{{Value: 0.125, Timestamp: 1700000000000}},
		},
	}
}

func writesFor(c *stateCache, result string) float64 {
	return testutil.ToFloat64(c.metrics.writes.WithLabelValues(result))
}

func readsFor(c *stateCache, result string) float64 {
	return testutil.ToFloat64(c.metrics.reads.WithLabelValues(result))
}

// histogramSamples returns the observation count and sum of the named histogram,
// read through the registry the way a scrape would see it.
func histogramSamples(t *testing.T, reg prometheus.Gatherer, name string) (uint64, float64) {
	t.Helper()

	mfs, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == name {
			require.Len(t, mf.GetMetric(), 1)

			h := mf.GetMetric()[0].GetHistogram()

			return h.GetSampleCount(), h.GetSampleSum()
		}
	}

	t.Fatalf("%s not registered", name)

	return 0, 0
}

func recordBytesCount(t *testing.T, reg prometheus.Gatherer) uint64 {
	t.Helper()

	count, _ := histogramSamples(t, reg, "sm_agent_scraper_cache_record_bytes")

	return count
}

func TestStateKey(t *testing.T) {
	var check model.Check

	err := check.FromSM(sm.Check{
		Id:        42,
		TenantId:  1000,
		Frequency: 300000,
		Timeout:   1000,
		Modified:  1700000000.5,
		Enabled:   true,
		Target:    "127.0.0.1",
		Job:       "test",
		Settings:  sm.CheckSettings{Ping: &sm.PingSettings{}},
	})
	require.NoError(t, err)

	check.RegionId = 3

	want := "sm:scraper:v1:" + strconv.FormatInt(int64(check.GlobalID()), 10) + ":" + check.ConfigVersion()

	require.Equal(t, want, stateKey(check))
	require.NotEqual(t, "sm:scraper:v1:42:"+check.ConfigVersion(), stateKey(check), "the key must carry the global ID, not the local one")
}

func TestStatePayloadRoundTrip(t *testing.T) {
	w := NewStateCache(&fakeStateStore{}, prometheus.NewPedanticRegistry(), zerolog.Nop())
	in := stateTestSeries()

	encoded, err := w.encode(in)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	out, err := w.decode(encoded)
	require.NoError(t, err)
	require.Equal(t, in, out)

	_, err = w.decode([]byte("not zstd"))
	require.Error(t, err)
}

func TestNoopStateCache(t *testing.T) {
	var c StateCache = NoopStateCache{}

	check := stateTestCheck(t, 5*time.Minute)

	// The noop has no store and no registry: Store returns, Load never
	// finds anything.
	c.Store(context.Background(), check, time.Now().Add(5*time.Minute), stateTestSeries())

	_, ts, ok := c.Load(context.Background(), check)
	require.False(t, ok)
	require.Nil(t, ts)
}

func TestStateCacheStore(t *testing.T) {
	type testcase struct {
		frequency   time.Duration
		firedAtAgo  time.Duration
		storeErr    error
		wantSet     bool
		wantResults map[string]float64
	}

	testcases := map[string]testcase{
		"5m check writes one record": {
			frequency:   5 * time.Minute,
			wantSet:     true,
			wantResults: map[string]float64{"ok": 1},
		},
		"record already expired is skipped": {
			frequency:   5 * time.Minute,
			firedAtAgo:  8 * time.Minute,
			wantResults: map[string]float64{"skipped_expired": 1},
		},
		"store error is counted": {
			frequency:   5 * time.Minute,
			storeErr:    errors.New("boom"),
			wantSet:     true,
			wantResults: map[string]float64{"error": 1},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			store := &fakeStateStore{err: tc.storeErr}

			reg := prometheus.NewPedanticRegistry()
			w := NewStateCache(store, reg, zerolog.Nop())

			check := stateTestCheck(t, tc.frequency)
			series := stateTestSeries()
			firedAt := time.Now().Add(-tc.firedAtAgo)

			w.Store(context.Background(), check, firedAt.Add(tc.frequency), series)

			if !tc.wantSet {
				require.Empty(t, store.calls)
				require.Zero(t, recordBytesCount(t, reg))
			} else {
				require.Len(t, store.calls, 1)
				require.Equal(t, stateKey(check), store.calls[0].key)

				record, ok := store.calls[0].value.(StateRecord)
				require.True(t, ok, "value is %T, want StateRecord", store.calls[0].value)
				require.Equal(t, firedAt.Add(tc.frequency), record.NextFire)

				decoded, err := w.decode(record.Payload)
				require.NoError(t, err)
				require.Equal(t, series, decoded)

				wantTTL := tc.frequency + stateSlack
				require.LessOrEqual(t, store.calls[0].ttl, wantTTL)
				require.Greater(t, store.calls[0].ttl, wantTTL-2*time.Second)

				require.Equal(t, uint64(1), recordBytesCount(t, reg))
			}

			for _, result := range []string{"ok", "error", "skipped_expired"} {
				require.Equal(t, tc.wantResults[result], writesFor(w, result), "result %q", result)
			}

			problems, err := testutil.GatherAndLint(reg)
			require.NoError(t, err)
			require.Empty(t, problems)
		})
	}
}

// benchPayloadSizes are the browser-check remote-write payload sizes measured
// in production in 2026-10 (p50, p90, p99; the runner caps metrics at 100 KiB).
// Browser checks have the largest payloads, so these bound the codec cost.
var benchPayloadSizes = []struct {
	name   string
	target int
}{
	{name: "p50_10KiB", target: 10752},
	{name: "p90_38KiB", target: 38912},
	{name: "p99_94KiB", target: 96768},
}

// benchStateSeries builds series shaped like a check's remote-write output
// until the marshaled request reaches target bytes, and returns that size.
func benchStateSeries(b *testing.B, target int) (TimeSeries, int) {
	b.Helper()

	var ts TimeSeries

	for {
		n := len(ts)

		ts = append(ts, prompb.TimeSeries{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "probe_browser_web_vital_lcp"},
				{Name: "check_name", Value: "browser"},
				{Name: "config_version", Value: "1700000000500000000-1"},
				{Name: "instance", Value: "https://example.com/checkout/step/" + strconv.Itoa(n)},
				{Name: "job", Value: "benchmark job"},
				{Name: "probe", Value: "Frankfurt"},
				{Name: "scenario", Value: "scenario_" + strconv.Itoa(n%6)},
			},
			Samples: []prompb.Sample{{Value: float64(n) * 0.001, Timestamp: 1700000000000}},
		})

		if size := (&prompb.WriteRequest{Timeseries: ts}).Size(); size >= target {
			return ts, size
		}
	}
}

func BenchmarkEncodeStatePayload(b *testing.B) {
	w := NewStateCache(&fakeStateStore{}, prometheus.NewPedanticRegistry(), zerolog.Nop())

	for _, bc := range benchPayloadSizes {
		b.Run(bc.name, func(b *testing.B) {
			ts, size := benchStateSeries(b, bc.target)

			encoded, err := w.encode(ts)
			if err != nil {
				b.Fatal(err)
			}

			b.SetBytes(int64(size))

			for b.Loop() {
				if _, err := w.encode(ts); err != nil {
					b.Fatal(err)
				}
			}

			// After the loop: Loop resets the timer and the custom metrics with it.
			b.ReportMetric(float64(size)/float64(len(encoded)), "ratio")
		})
	}

	// The writer shares one encoder state between all scrapers, so concurrent
	// encodes queue behind each other; this measures the throughput of that queue.
	b.Run("p90_38KiB_parallel", func(b *testing.B) {
		ts, size := benchStateSeries(b, benchPayloadSizes[1].target)

		b.SetBytes(int64(size))

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if _, err := w.encode(ts); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

func BenchmarkDecodeStatePayload(b *testing.B) {
	w := NewStateCache(&fakeStateStore{}, prometheus.NewPedanticRegistry(), zerolog.Nop())

	for _, bc := range benchPayloadSizes {
		b.Run(bc.name, func(b *testing.B) {
			ts, size := benchStateSeries(b, bc.target)

			encoded, err := w.encode(ts)
			if err != nil {
				b.Fatal(err)
			}

			b.SetBytes(int64(size))

			for b.Loop() {
				if _, err := w.decode(encoded); err != nil {
					b.Fatal(err)
				}
			}
		})
	}

	// Same single decoder state as the encoder: this is the queue a pod restart
	// forms when every scraper loads its record within the same few seconds.
	b.Run("p90_38KiB_parallel", func(b *testing.B) {
		ts, size := benchStateSeries(b, benchPayloadSizes[1].target)

		encoded, err := w.encode(ts)
		if err != nil {
			b.Fatal(err)
		}

		b.SetBytes(int64(size))

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if _, err := w.decode(encoded); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

func TestStateCacheLoad(t *testing.T) {
	check := stateTestCheck(t, 5*time.Minute)
	nextFire := time.Now().Add(4 * time.Minute)

	validPayload, err := NewStateCache(&fakeStateStore{}, prometheus.NewPedanticRegistry(), zerolog.Nop()).encode(stateTestSeries())
	require.NoError(t, err)

	type testcase struct {
		records     map[string]StateRecord
		getErr      error
		wantOK      bool
		wantResults map[string]float64
	}

	testcases := map[string]testcase{
		"record present": {
			records:     map[string]StateRecord{stateKey(check): {NextFire: nextFire, Payload: validPayload}},
			wantOK:      true,
			wantResults: map[string]float64{"hit": 1},
		},
		"record absent is a miss": {
			wantResults: map[string]float64{"miss": 1},
		},
		"store error is counted": {
			getErr:      errors.New("boom"),
			wantResults: map[string]float64{"error": 1},
		},
		"garbage payload is a decode error": {
			records:     map[string]StateRecord{stateKey(check): {NextFire: nextFire, Payload: []byte("not zstd")}},
			wantResults: map[string]float64{"decode_error": 1},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			store := &fakeStateStore{records: tc.records, getErr: tc.getErr}

			reg := prometheus.NewPedanticRegistry()
			c := NewStateCache(store, reg, zerolog.Nop())

			rec, ts, ok := c.Load(context.Background(), check)

			require.Len(t, store.gets, 1)
			require.Equal(t, stateKey(check), store.gets[0].key)
			require.Equal(t, tc.wantOK, ok)

			if tc.wantOK {
				require.True(t, nextFire.Equal(rec.NextFire))
				require.Equal(t, validPayload, rec.Payload)
				require.Equal(t, stateTestSeries(), ts)
			} else {
				require.Nil(t, ts)
			}

			for _, result := range stateReadResults {
				require.Equal(t, tc.wantResults[result], readsFor(c, result), "result %q", result)
			}

			problems, err := testutil.GatherAndLint(reg)
			require.NoError(t, err)
			require.Empty(t, problems)
		})
	}
}

// TestStateRecordRoundTripThroughLocalCache pins that a StateRecord survives the
// gob encoding the real caches apply, which is what the fake store skips.
func TestStateRecordRoundTripThroughLocalCache(t *testing.T) {
	local, err := cache.NewLocal(cache.LocalConfig{MaxCapacity: 10, Logger: zerolog.Nop()})
	require.NoError(t, err)

	check := stateTestCheck(t, 5*time.Minute)
	c := NewStateCache(local, prometheus.NewPedanticRegistry(), zerolog.Nop())

	firedAt := time.Now()
	c.Store(context.Background(), check, firedAt.Add(5*time.Minute), stateTestSeries())
	require.Equal(t, 1.0, writesFor(c, "ok"))

	rec, ts, ok := c.Load(context.Background(), check)
	require.True(t, ok)
	require.True(t, firedAt.Add(5*time.Minute).Equal(rec.NextFire), "got %s", rec.NextFire)
	require.Equal(t, stateTestSeries(), ts)

	var missing StateRecord

	require.ErrorIs(t, local.Get(context.Background(), stateKeyPrefix+"missing", &missing), cache.ErrCacheMiss)
}

// gatedStateStore holds every Get until the gate opens and records how many
// were in flight at once, which is what the reader semaphore is supposed to
// bound. It is safe for concurrent use, unlike fakeStateStore.
type gatedStateStore struct {
	gate     chan struct{}
	inflight atomic.Int32
	peak     atomic.Int32
	calls    atomic.Int32
}

func newGatedStateStore() *gatedStateStore {
	return &gatedStateStore{gate: make(chan struct{})}
}

func (s *gatedStateStore) Set(context.Context, string, any, time.Duration) error { return nil }

func (s *gatedStateStore) Get(ctx context.Context, _ string, _ any) error {
	s.calls.Add(1)

	n := s.inflight.Add(1)
	defer s.inflight.Add(-1)

	for {
		p := s.peak.Load()
		if n <= p || s.peak.CompareAndSwap(p, n) {
			break
		}
	}

	select {
	case <-s.gate:
		return cache.ErrCacheMiss
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestStateCacheLoadBoundsConcurrency(t *testing.T) {
	const callers = 4 * maxConcurrentReaders

	store := newGatedStateStore()
	c := NewStateCache(store, prometheus.NewPedanticRegistry(), zerolog.Nop())
	check := stateTestCheck(t, 5*time.Minute)

	var wg sync.WaitGroup

	for range callers {
		wg.Go(func() {
			_, _, ok := c.Load(context.Background(), check)
			require.False(t, ok)
		})
	}

	// Let the first wave reach the store, then confirm nobody beyond the
	// permit count has.
	require.Eventually(t, func() bool { return store.inflight.Load() == maxConcurrentReaders }, time.Second, time.Millisecond)
	require.Equal(t, int32(maxConcurrentReaders), store.calls.Load(), "callers without a permit must not reach the store")

	close(store.gate)
	wg.Wait()

	require.Equal(t, int32(maxConcurrentReaders), store.peak.Load())
	require.Equal(t, int32(callers), store.calls.Load(), "every caller gets its turn once the gate opens")
	require.Equal(t, float64(callers), readsFor(c, "miss"))
}

func TestStateCacheLoadCancelledWhileWaiting(t *testing.T) {
	store := newGatedStateStore()
	c := NewStateCache(store, prometheus.NewPedanticRegistry(), zerolog.Nop())
	check := stateTestCheck(t, 5*time.Minute)

	// Fill every permit with callers parked on the gate.
	var holders sync.WaitGroup

	for range maxConcurrentReaders {
		holders.Go(func() { c.Load(context.Background(), check) })
	}

	require.Eventually(t, func() bool { return store.inflight.Load() == maxConcurrentReaders }, time.Second, time.Millisecond)

	// A caller that gives up in the queue never reaches the store and is
	// counted, so the read does not disappear from the metrics.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, ok := c.Load(ctx, check)

	require.False(t, ok)
	require.Less(t, time.Since(start), time.Second)
	require.Equal(t, int32(maxConcurrentReaders), store.calls.Load())
	require.Equal(t, 1.0, readsFor(c, "error"))

	// And one whose context is already over fails in the queue like any other cancelled caller.
	done, cancelDone := context.WithCancel(context.Background())
	cancelDone()

	_, _, ok = c.Load(done, check)

	require.False(t, ok)
	require.Equal(t, int32(maxConcurrentReaders), store.calls.Load())
	require.Equal(t, 2.0, readsFor(c, "error"))

	close(store.gate)
	holders.Wait()
}
