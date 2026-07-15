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

func pingCheck() sm.Check {
	check := testCheck().Check
	check.Settings = sm.CheckSettings{Ping: &sm.PingSettings{}}
	check.Target = "8.8.8.8"
	return check
}

// TestSyntheticPingProberFixtureSuperset is the objective acceptance gate: the
// series names CollectTyped emits for a successful ping check must exactly
// match the series names declared by the oracle fixture
// internal/scraper/testdata/ping.txt.
func TestSyntheticPingProberFixtureSuperset(t *testing.T) {
	ctx := context.Background()
	check := pingCheck()
	probe := testProbe()

	gen, err := backfill.NewGeneratorForCheck(ctx, check, probe)
	require.NoError(t, err)

	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.NewTypedPingSample(backfill.PingSample{
		Success:          true,
		DNSLookupSeconds: 0.000005542,
		DurationSeconds:  0.00014434799999999998,
		RTTMin:           0.0000710840,
		RTTMax:           0.0000910420,
		RTTStddev:        0.0000083350,
		ICMPDuration:     0.00008230600,
		ReplyHopLimit:    64,
	})

	ts, streams, err := gen.CollectTyped(ctx, at, sample, "exec-ping-1")
	require.NoError(t, err)
	require.NotEmpty(t, ts)
	require.NotEmpty(t, streams)

	got := metricNames(ts)
	want := fixtureMetricNames(t, "../../internal/scraper/testdata/ping.txt")
	require.ElementsMatch(t, want, got)
}

func TestSyntheticPingProberEmitLogsSuccess(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.PingSample{
		At:               at,
		Success:          true,
		DNSLookupSeconds: 0.000005542,
		DurationSeconds:  0.00014434799999999998,
		RTTMin:           0.0000710840,
		RTTMax:           0.0000910420,
		RTTStddev:        0.0000083350,
		ICMPDuration:     0.00008230600,
		ReplyHopLimit:    64,
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticPingProber("8.8.8.8")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "8.8.8.8", registry, logger, "exec-1")
	require.True(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 5, "success path should emit exactly 5 log lines")

	for _, line := range lines {
		require.Contains(t, line, "level=INFO")
	}

	joined := strings.Join(lines, "\n")
	require.Contains(t, joined, `msg="Creating socket"`)
	require.Contains(t, joined, `msg="Using source address"`)
	require.Contains(t, joined, `msg="Creating ICMP packet"`)
	require.Contains(t, joined, `msg="Waiting for reply packets"`)
	require.Contains(t, joined, `msg="Probe finished"`)
	require.Contains(t, joined, `packets_sent=3`)
	require.Contains(t, joined, `packets_received=3`)
}

func TestSyntheticPingProberEmitLogsTimeout(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.PingSample{
		At:      at,
		Success: false,
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticPingProber("8.8.8.8")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "8.8.8.8", registry, logger, "exec-1")
	require.False(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 5, "timeout path should emit exactly 5 log lines")
	// Production logs "failed to run ping" at INFO, not ERROR
	// (internal/prober/icmp/icmp_impl.go:149) -- mirrored here, quirk and all.
	for _, line := range lines {
		require.Contains(t, line, "level=INFO")
	}
	require.Contains(t, lines[4], `msg="failed to run ping"`)
}

// TestSyntheticPingProberLossEmitsZeroPacketsReceived pins the I2 fix
// end-to-end: a total-loss execution (Success=false, packet counts left for
// Normalize to fill) must emit probe_icmp_packets_received_count=0, not the
// pre-fix 3 (which made a 100%-loss window look like 0% packet loss to any
// panel computed from packets_received/packets_sent).
func TestSyntheticPingProberLossEmitsZeroPacketsReceived(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.PingSample{At: at, Success: false}

	prober := backfill.NewSyntheticPingProber("8.8.8.8")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "8.8.8.8", registry, log.NewLogfmtLogger(io.Discard), "exec-1")
	require.False(t, success)

	families, err := registry.Gather()
	require.NoError(t, err)

	var sentValue, receivedValue float64
	var sawSent, sawReceived bool
	for _, mf := range families {
		switch mf.GetName() {
		case "probe_icmp_packets_sent_count":
			sentValue = mf.GetMetric()[0].GetGauge().GetValue()
			sawSent = true
		case "probe_icmp_packets_received_count":
			receivedValue = mf.GetMetric()[0].GetGauge().GetValue()
			sawReceived = true
		}
	}
	require.True(t, sawSent, "probe_icmp_packets_sent_count must be registered")
	require.True(t, sawReceived, "probe_icmp_packets_received_count must be registered")
	require.Equal(t, 3.0, sentValue, "a lost probe still attempted the full packet count")
	require.Equal(t, 0.0, receivedValue, "a lost probe receives nothing back")
}

func TestSyntheticPingProberName(t *testing.T) {
	require.Equal(t, "ping", backfill.NewSyntheticPingProber("8.8.8.8").Name())
}

func TestSyntheticPingProberSetTypedIgnoresMismatchedType(t *testing.T) {
	prober := backfill.NewSyntheticPingProber("8.8.8.8")
	prober.SetTyped(backfill.NewTypedHTTPSample(testSample(time.Now())))
	// Should not panic and should leave the ping sample zero-valued; Probe
	// still runs (Normalize fills defaults) without error.
	registry := prometheus.NewRegistry()
	logger := log.NewLogfmtLogger(io.Discard)
	success, _ := prober.Probe(context.Background(), "8.8.8.8", registry, logger, "exec-1")
	require.False(t, success)
}
