package scraper

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

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

// fakeStateStore records Set calls. With blockUntilDone set, Set waits for the
// context to end and returns its error, standing in for a cache that hangs.
type fakeStateStore struct {
	calls          []stateSetCall
	err            error
	blockUntilDone bool
}

func (s *fakeStateStore) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	err := s.err

	if s.blockUntilDone {
		<-ctx.Done()
		err = ctx.Err()
	}

	s.calls = append(s.calls, stateSetCall{key: key, value: value, ttl: ttl, err: err})

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

func writesFor(w *StateWriter, result string) float64 {
	return testutil.ToFloat64(w.metrics.writes.WithLabelValues(result))
}

// recordBytesCount returns the number of observations in the record_bytes
// histogram, read through the registry the way a scrape would see it.
func recordBytesCount(t *testing.T, reg prometheus.Gatherer) uint64 {
	t.Helper()

	mfs, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "sm_agent_scraper_cache_record_bytes" {
			require.Len(t, mf.GetMetric(), 1)

			return mf.GetMetric()[0].GetHistogram().GetSampleCount()
		}
	}

	t.Fatal("sm_agent_scraper_cache_record_bytes not registered")

	return 0
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
	in := stateTestSeries()

	encoded, err := encodeStatePayload(in)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	out, err := decodeStatePayload(encoded)
	require.NoError(t, err)
	require.Equal(t, in, out)

	_, err = decodeStatePayload([]byte("not zstd"))
	require.Error(t, err)
}

func TestStateWriterDisabled(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	w := NewStateWriter(nil, reg, zerolog.Nop())

	require.False(t, w.Enabled())

	w.Store(context.Background(), stateTestCheck(t, 5*time.Minute), time.Now(), stateTestSeries())

	mfs, err := reg.Gather()
	require.NoError(t, err)
	require.Empty(t, mfs, "a disabled writer registers no metrics")
}

func TestStateWriterStore(t *testing.T) {
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
			w := NewStateWriter(store, reg, zerolog.Nop())
			require.True(t, w.Enabled())

			check := stateTestCheck(t, tc.frequency)
			series := stateTestSeries()
			firedAt := time.Now().Add(-tc.firedAtAgo)

			w.Store(context.Background(), check, firedAt, series)

			if !tc.wantSet {
				require.Empty(t, store.calls)
				require.Zero(t, recordBytesCount(t, reg))
			} else {
				require.Len(t, store.calls, 1)
				require.Equal(t, stateKey(check), store.calls[0].key)

				record, ok := store.calls[0].value.(StateRecord)
				require.True(t, ok, "value is %T, want StateRecord", store.calls[0].value)
				require.Equal(t, firedAt.Add(tc.frequency), record.NextFire)

				decoded, err := decodeStatePayload(record.Payload)
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

func TestStateWriterStoreTimesOut(t *testing.T) {
	store := &fakeStateStore{blockUntilDone: true}

	w := NewStateWriter(store, prometheus.NewPedanticRegistry(), zerolog.Nop())
	w.timeout = 20 * time.Millisecond

	start := time.Now()

	w.Store(context.Background(), stateTestCheck(t, 5*time.Minute), time.Now(), stateTestSeries())

	require.Less(t, time.Since(start), time.Second, "Store must return once the write timeout fires")
	require.Len(t, store.calls, 1)
	require.ErrorIs(t, store.calls[0].err, context.DeadlineExceeded)
	require.Equal(t, 1.0, writesFor(w, "error"))
	require.Equal(t, 0.0, writesFor(w, "ok"))
}

func TestStateWriterStoreCancelledContext(t *testing.T) {
	store := &fakeStateStore{blockUntilDone: true}

	w := NewStateWriter(store, prometheus.NewPedanticRegistry(), zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()

	w.Store(ctx, stateTestCheck(t, 5*time.Minute), time.Now(), stateTestSeries())

	require.Less(t, time.Since(start), w.timeout, "a cancelled context must not wait for the write timeout")
	require.Len(t, store.calls, 1)
	require.ErrorIs(t, store.calls[0].err, context.Canceled)
	require.Equal(t, 1.0, writesFor(w, "error"))
	require.Equal(t, 0.0, writesFor(w, "ok"))
}
