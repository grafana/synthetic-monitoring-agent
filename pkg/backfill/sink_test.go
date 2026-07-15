package backfill_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	logproto "github.com/grafana/loki/pkg/push"
	"github.com/grafana/synthetic-monitoring-agent/pkg/backfill"
	"github.com/stretchr/testify/require"
)

func TestConsolidateStreamsMergesExecutionStreams(t *testing.T) {
	t1 := time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)

	streams := backfill.Streams{
		{
			Labels: `{job="backfill-http-latency", execution_id="exec-a", probe="probe-1"}`,
			Entries: []logproto.Entry{
				{Timestamp: t2, Line: `msg="Beginning check"`},
			},
		},
		{
			Labels: `{job="backfill-http-latency", execution_id="exec-b", probe="probe-1"}`,
			Entries: []logproto.Entry{
				{Timestamp: t1, Line: `msg="Beginning check"`},
			},
		},
	}

	out := backfill.ConsolidateStreams(streams)
	require.Len(t, out, 1)
	require.NotContains(t, out[0].Labels, "execution_id")
	require.Len(t, out[0].Entries, 2)
	require.Equal(t, t1, out[0].Entries[0].Timestamp)
	require.Equal(t, t2, out[0].Entries[1].Timestamp)
}

func TestWriteLokiPushBatchedUsesMultiplePushes(t *testing.T) {
	var pushCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/loki/api/v1/push" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.Copy(io.Discard, r.Body)
		pushCount.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	base := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	streams := make(backfill.Streams, 0, 50)
	for i := range 50 {
		ts := base.Add(time.Duration(i) * time.Minute)
		streams = append(streams, logproto.Stream{
			Labels: fmt.Sprintf(`{job="backfill-http-latency", execution_id="exec-%d", probe="probe-1"}`, i),
			Entries: []logproto.Entry{
				{Timestamp: ts, Line: `msg="Beginning check"`},
				{Timestamp: ts.Add(200 * time.Millisecond), Line: `duration_seconds=0.2 msg="Check succeeded"`},
			},
		})
	}

	err := backfill.WriteLokiPushBatched(context.Background(), srv.URL, streams, &backfill.LokiPushBatchOptions{
		MaxStreamsPerPush: 20,
		MaxEntriesPerPush: 1000,
		MaxWindowPerPush:  time.Hour,
		PauseBetween:      0,
	}, srv.Client())
	require.NoError(t, err)
	require.GreaterOrEqual(t, int(pushCount.Load()), 3)
}
