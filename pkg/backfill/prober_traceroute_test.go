package backfill_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/pkg/backfill"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func tracerouteCheck() sm.Check {
	check := testCheck().Check
	check.Settings = sm.CheckSettings{Traceroute: &sm.TracerouteSettings{}}
	check.Target = "grafana.com"
	return check
}

// TestSyntheticTracerouteProberFixtureSuperset is the objective acceptance
// gate: the series names CollectTyped emits for a successful traceroute
// check must exactly match the series names declared by the oracle fixture
// internal/scraper/testdata/traceroute.txt (probe_traceroute_total_hops,
// probe_traceroute_route_hash, probe_traceroute_packet_loss_percent, plus the
// envelope).
func TestSyntheticTracerouteProberFixtureSuperset(t *testing.T) {
	ctx := context.Background()
	check := tracerouteCheck()
	probe := testProbe()

	gen, err := backfill.NewGeneratorForCheck(ctx, check, probe)
	require.NoError(t, err)

	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.NewTypedTracerouteSample(backfill.TracerouteSample{
		Success: true,
		Hops: []backfill.TracerouteHop{
			{Host: "10.0.0.1", RTTSeconds: 0.001},
			{Host: "10.0.0.2", RTTSeconds: 0.002},
		},
	})

	ts, streams, err := gen.CollectTyped(ctx, at, sample, "exec-traceroute-1")
	require.NoError(t, err)
	require.NotEmpty(t, ts)
	require.NotEmpty(t, streams)

	got := metricNames(ts)
	want := fixtureMetricNames(t, "../../internal/scraper/testdata/traceroute.txt")
	require.ElementsMatch(t, want, got)
}

func TestSyntheticTracerouteProberName(t *testing.T) {
	require.Equal(t, "traceroute", backfill.NewSyntheticTracerouteProber("grafana.com").Name())
}

// TestSyntheticTracerouteProberEmitLogsHopLineShape proves the per-hop log
// lines mirror the real check's exact logfmt keys
// (internal/prober/traceroute/traceroute.go: the logger.Log call inside the
// ttl loop): "Level", "Destination", "Hosts", "TTL", "ElapsedTime",
// "LossPercent", "Sent", "TracerouteID". Notably the real check does not
// include "msg" or lowercase "level"/"time" keys on these lines (unlike the
// other synthetic probers), so this test asserts their absence too.
func TestSyntheticTracerouteProberEmitLogsHopLineShape(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.TracerouteSample{
		At:      at,
		Success: true,
		Hops: []backfill.TracerouteHop{
			{Host: "10.0.0.1", RTTSeconds: 0.001},
			{Host: "10.0.0.2", RTTSeconds: 0.012, Loss: true},
		},
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticTracerouteProber("grafana.com")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "grafana.com", registry, logger, "exec-1")
	require.True(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, backfill.TracerouteLogLineCount(2), "2-hop success emits exactly hops lines (no trailing summary line -- production has none)")

	hop1, hop2 := lines[0], lines[1]

	for _, key := range []string{"Level=info", "Destination=grafana.com", "Hosts=", "TTL=", "ElapsedTime=", "LossPercent=", "Sent=", "TracerouteID="} {
		require.Contains(t, hop1, key, "hop line missing key %q: %q", key, hop1)
	}
	require.NotContains(t, hop1, "msg=")

	require.Contains(t, hop1, "Hosts=10.0.0.1")
	require.Contains(t, hop1, "TTL=1")
	require.Contains(t, hop1, "LossPercent=0")

	require.Contains(t, hop2, "Hosts=10.0.0.2")
	require.Contains(t, hop2, "TTL=2")
	require.Contains(t, hop2, "LossPercent=100")
}

func TestSyntheticTracerouteProberEmitLogsRunFailure(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.TracerouteSample{
		At:      at,
		Success: false,
		Hops: []backfill.TracerouteHop{
			{Host: "10.0.0.1", RTTSeconds: 0.001},
		},
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticTracerouteProber("grafana.com")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "grafana.com", registry, logger, "exec-1")
	require.False(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, backfill.TracerouteLogLineCount(1)+1, "failure adds one leading error line")
	require.Contains(t, lines[0], "Level=error")
	require.Contains(t, lines[0], "msg=")
}

func TestTracerouteLogLineCount(t *testing.T) {
	// Production emits exactly one log line per hop and no trailing summary
	// line (see emitLogs' doc comment), so the count is just the hop count.
	require.Equal(t, 0, backfill.TracerouteLogLineCount(0))
	require.Equal(t, 3, backfill.TracerouteLogLineCount(3))
	require.Equal(t, 10, backfill.TracerouteLogLineCount(10))
}

func TestSyntheticTracerouteProberSetTypedIgnoresMismatchedType(t *testing.T) {
	prober := backfill.NewSyntheticTracerouteProber("grafana.com")
	prober.SetTyped(backfill.NewTypedHTTPSample(testSample(time.Now())))
	// Should not panic and should leave the traceroute sample zero-valued
	// (Success false); Probe still runs (Normalize synthesizes hops) without
	// error.
	registry := prometheus.NewRegistry()
	logger := log.NewLogfmtLogger(io.Discard)
	success, _ := prober.Probe(context.Background(), "grafana.com", registry, logger, "exec-1")
	require.False(t, success)
}
