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

func tcpCheck() sm.Check {
	check := testCheck().Check
	check.Settings = sm.CheckSettings{Tcp: &sm.TcpSettings{}}
	check.Target = "example.com:443"
	return check
}

// TestSyntheticTCPProberFixtureSuperset is the objective acceptance gate: the
// series names CollectTyped emits for a successful TCP check must exactly
// match the series names declared by the oracle fixture
// internal/scraper/testdata/tcp.txt.
func TestSyntheticTCPProberFixtureSuperset(t *testing.T) {
	ctx := context.Background()
	check := tcpCheck()
	probe := testProbe()

	gen, err := backfill.NewGeneratorForCheck(ctx, check, probe)
	require.NoError(t, err)

	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.NewTypedTCPSample(backfill.TCPSample{
		Success:          true,
		DNSLookupSeconds: 0.0000071250,
		DurationSeconds:  0.000449208,
	})

	ts, streams, err := gen.CollectTyped(ctx, at, sample, "exec-tcp-1")
	require.NoError(t, err)
	require.NotEmpty(t, ts)
	require.NotEmpty(t, streams)

	got := metricNames(ts)
	want := fixtureMetricNames(t, "../../internal/scraper/testdata/tcp.txt")
	require.ElementsMatch(t, want, got)
}

// TestSyntheticTCPProberFixtureSupersetSSL verifies that SSL-enabled TCP checks
// emit the additional SSL metrics declared by tcp_ssl.txt.
func TestSyntheticTCPProberFixtureSupersetSSL(t *testing.T) {
	ctx := context.Background()
	check := tcpCheck()
	probe := testProbe()

	gen, err := backfill.NewGeneratorForCheck(ctx, check, probe)
	require.NoError(t, err)

	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.NewTypedTCPSample(backfill.TCPSample{
		Success:                            true,
		DNSLookupSeconds:                   0.0000064580,
		DurationSeconds:                    0.020992375,
		SSL:                                true,
		SSLEarliestCertExpiry:              3.6e9,
		SSLLastChainExpiryTimestampSeconds: -6.21355968e10,
		SSLLastChainFingerprint:            "efc04a3afb86376b3a4db1b1d2f454afc60d192a573d78541836d83e4c849813",
		SSLLastChainIssuer:                 "O=Acme Co",
		SSLLastChainSerialNumber:           "8a086bc8a70f8a416a58b6741a5cebec",
		SSLLastChainSubject:                "O=Acme Co",
		SSLLastChainSubjectAlternative:     "example.com",
		TLSVersion:                         "TLS 1.3",
	})

	ts, streams, err := gen.CollectTyped(ctx, at, sample, "exec-tcp-ssl-1")
	require.NoError(t, err)
	require.NotEmpty(t, ts)
	require.NotEmpty(t, streams)

	got := metricNames(ts)
	want := fixtureMetricNames(t, "../../internal/scraper/testdata/tcp_ssl.txt")
	require.ElementsMatch(t, want, got)
}

func TestSyntheticTCPProberEmitLogsSuccess(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.TCPSample{
		At:              at,
		Success:         true,
		DurationSeconds: 0.05,
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticTCPProber("example.com:443")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "example.com:443", registry, logger, "exec-1")
	require.True(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 4, "success path should emit exactly 4 log lines")

	for _, line := range lines {
		require.Contains(t, line, "level=INFO")
	}

	joined := strings.Join(lines, "\n")
	require.Contains(t, joined, `msg="Resolving target address"`)
	require.Contains(t, joined, `msg="Resolved target address"`)
	require.Contains(t, joined, `msg="Dialing TCP without TLS"`)
	require.Contains(t, joined, `msg="Successfully dialed"`)
}

func TestSyntheticTCPProberEmitLogsSuccessWithExpect(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.TCPSample{
		At:              at,
		Success:         true,
		DurationSeconds: 0.05,
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticTCPProber("example.com:443")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "example.com:443", registry, logger, "exec-1")
	require.True(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 4, "success path should emit exactly 4 log lines (resolving, resolved, dialing, successfully dialed)")
}

func TestSyntheticTCPProberEmitLogsDialFailed(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.TCPSample{
		At:               at,
		Success:          false,
		FailedDueToRegex: 0,
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticTCPProber("example.com:443")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "example.com:443", registry, logger, "exec-1")
	require.False(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 4, "dial-failure path should emit exactly 4 log lines")
	require.Contains(t, lines[0], "level=INFO")
	require.Contains(t, lines[1], "level=INFO")
	require.Contains(t, lines[2], "level=INFO")
	require.Contains(t, lines[3], "level=ERROR")
	require.Contains(t, lines[3], `msg="Error dialing TCP"`)
}

func TestSyntheticTCPProberEmitLogsExpectMismatch(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.TCPSample{
		At:               at,
		Success:          false,
		FailedDueToRegex: 1,
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticTCPProber("example.com:443")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "example.com:443", registry, logger, "exec-1")
	require.False(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 6, "expect-mismatch path should emit exactly 6 log lines")
	require.Contains(t, lines[0], "level=INFO")
	require.Contains(t, lines[1], "level=INFO")
	require.Contains(t, lines[2], "level=INFO")
	require.Contains(t, lines[3], "level=INFO")
	require.Contains(t, lines[4], "level=INFO")
	require.Contains(t, lines[5], "level=ERROR")
	require.Contains(t, lines[5], `msg="Regexp did not match"`)
}

func TestSyntheticTCPProberName(t *testing.T) {
	require.Equal(t, "tcp", backfill.NewSyntheticTCPProber("example.com:443").Name())
}

func TestSyntheticTCPProberSetTypedIgnoresMismatchedType(t *testing.T) {
	prober := backfill.NewSyntheticTCPProber("example.com:443")
	prober.SetTyped(backfill.NewTypedHTTPSample(testSample(time.Now())))
	// Should not panic and should leave the TCP sample zero-valued; Probe
	// still runs (Normalize fills defaults) without error.
	registry := prometheus.NewRegistry()
	logger := log.NewLogfmtLogger(io.Discard)
	success, _ := prober.Probe(context.Background(), "example.com:443", registry, logger, "exec-1")
	require.False(t, success)
}
