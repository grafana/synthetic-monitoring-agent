package k6runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestHttpRunnerDrainTriggersImmediateRetry(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	var firstAttemptAt, secondAttemptAt time.Time

	mux := http.NewServeMux()
	mux.HandleFunc("/run", func(w http.ResponseWriter, _ *http.Request) {
		switch attempts.Add(1) {
		case 1:
			firstAttemptAt = time.Now()
			w.Header().Set(DrainHeader, "1")
			w.Header().Set("Retry-After", "0")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(RunResponse{ErrorCode: ErrorCodeDispatcherDrain})
		default:
			secondAttemptAt = time.Now()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	reg := prometheus.NewRegistry()
	logger := zerolog.Nop()
	runner := HttpRunner{
		url: srv.URL, logger: &logger,
		// Backoff is 5s; if drain handling did not bypass it, the retry would arrive 5s later.
		graceTime: time.Second, backoff: 5 * time.Second,
		metrics: NewHTTPMetrics(reg),
	}

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	_, err := runner.Run(ctx, Script{Settings: Settings{Timeout: 500}, K6ChannelManifest: "test"}, SecretStore{})
	require.NoError(t, err)

	require.Equal(t, int32(2), attempts.Load())
	require.Less(t, secondAttemptAt.Sub(firstAttemptAt), 500*time.Millisecond,
		"drain retry should be near-immediate, not bounded by backoff")

	// Drain retry counter should have ticked exactly once.
	require.Equal(t, float64(1), testutil.ToFloat64(runner.metrics.DrainRetries))
	// And the retriable-failure counter should NOT have ticked for the drain attempt.
	got := testutil.ToFloat64(runner.metrics.Requests.With(prometheus.Labels{
		metricLabelSuccess: "0", metricLabelRetriable: "1",
	}))
	require.Equal(t, float64(0), got, "drain must not count as a retriable failure")
}

func TestGraceFor(t *testing.T) {
	t.Parallel()

	cases := map[string]time.Duration{
		"scripted":  GraceTimeSmall,
		"multihttp": GraceTimeSmall,
		"browser":   GraceTimeBrowser,
		"http":      defaultGraceTime,
		"":          defaultGraceTime,
	}
	for ct, want := range cases {
		require.Equal(t, want, GraceFor(ct), "checkType=%q", ct)
	}
}
