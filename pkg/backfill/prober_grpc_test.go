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

func grpcCheck() sm.Check {
	check := testCheck().Check
	check.Settings = sm.CheckSettings{Grpc: &sm.GrpcSettings{}}
	check.Target = "example.com:5051"
	return check
}

// TestSyntheticGRPCProberFixtureSuperset is the objective acceptance gate: the
// series names CollectTyped emits for a successful gRPC check must exactly
// match the series names declared by the oracle fixture
// internal/scraper/testdata/grpc_basic.txt.
func TestSyntheticGRPCProberFixtureSuperset(t *testing.T) {
	ctx := context.Background()
	check := grpcCheck()
	probe := testProbe()

	gen, err := backfill.NewGeneratorForCheck(ctx, check, probe)
	require.NoError(t, err)

	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.NewTypedGRPCSample(backfill.GRPCSample{
		Success:             true,
		DNSLookupSeconds:    0.000009875,
		GRPCDuration:        0.000571083,
		StatusCode:          0,
		HealthCheckResponse: 1, // SERVING
	})

	ts, streams, err := gen.CollectTyped(ctx, at, sample)
	require.NoError(t, err)
	require.NotEmpty(t, ts)
	require.NotEmpty(t, streams)

	got := metricNames(ts)
	want := fixtureMetricNames(t, "../../internal/scraper/testdata/grpc.txt")
	require.ElementsMatch(t, want, got)
}

// TestSyntheticGRPCProberFixtureSupersetSSL verifies that SSL-enabled gRPC checks
// emit the additional SSL metrics declared by grpc_ssl_basic.txt.
func TestSyntheticGRPCProberFixtureSupersetSSL(t *testing.T) {
	ctx := context.Background()
	check := grpcCheck()
	probe := testProbe()

	gen, err := backfill.NewGeneratorForCheck(ctx, check, probe)
	require.NoError(t, err)

	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.NewTypedGRPCSample(backfill.GRPCSample{
		Success:                        true,
		DNSLookupSeconds:               0.000004416,
		GRPCDuration:                   0.001843459,
		StatusCode:                     0,
		HealthCheckResponse:            1, // SERVING
		SSL:                            true,
		SSLEarliestCertExpiry:          3.6e9,
		SSLLastChainFingerprint:        "efc04a3afb86376b3a4db1b1d2f454afc60d192a573d78541836d83e4c849813",
		SSLLastChainIssuer:             "O=Acme Co",
		SSLLastChainSerialNumber:       "8a086bc8a70f8a416a58b6741a5cebec",
		SSLLastChainSubject:            "O=Acme Co",
		SSLLastChainSubjectAlternative: "example.com",
		TLSVersion:                     "TLS 1.3",
	})

	ts, streams, err := gen.CollectTyped(ctx, at, sample)
	require.NoError(t, err)
	require.NotEmpty(t, ts)
	require.NotEmpty(t, streams)

	got := metricNames(ts)
	want := fixtureMetricNames(t, "../../internal/scraper/testdata/grpc_ssl.txt")
	require.ElementsMatch(t, want, got)
}

func TestSyntheticGRPCProberEmitLogsSuccess(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.GRPCSample{
		At:                  at,
		Success:             true,
		DNSLookupSeconds:    0.0001,
		GRPCDuration:        0.05,
		HealthCheckResponse: 1, // SERVING
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticGRPCProber("example.com:5051")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "example.com:5051", registry, logger, "exec-1")
	require.True(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 3, "success path should emit exactly 3 log lines")

	require.Contains(t, lines[0], "level=INFO")
	require.Contains(t, lines[1], "level=INFO")
	// A successful check logs at DEBUG in production, not INFO -- see
	// emitLogs' doc comment.
	require.Contains(t, lines[2], "level=DEBUG")

	joined := strings.Join(lines, "\n")
	require.Contains(t, joined, `msg="Resolving target address"`)
	require.Contains(t, joined, `msg="Resolved target address"`)
	require.Contains(t, joined, `msg="connect the grpc server successfully"`)
}

func TestSyntheticGRPCProberEmitLogsSuccessWithTLS(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.GRPCSample{
		At:                  at,
		Success:             true,
		DNSLookupSeconds:    0.0001,
		GRPCDuration:        0.05,
		SSL:                 true,
		HealthCheckResponse: 1, // SERVING
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticGRPCProber("example.com:5051")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "example.com:5051", registry, logger, "exec-1")
	require.True(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// TLS status is never logged in production (it only affects
	// probe_grpc_ssl / probe_tls_version_info metrics), so SSL=true doesn't
	// change the log line count or shape.
	require.Len(t, lines, 3, "success path should emit exactly 3 log lines")
	require.Contains(t, lines[2], "level=DEBUG")
}

func TestSyntheticGRPCProberEmitLogsConnectionFailed(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.GRPCSample{
		At:            at,
		Success:       false,
		ConnectFailed: true,
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticGRPCProber("example.com:5051")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "example.com:5051", registry, logger, "exec-1")
	require.False(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 3, "connection-failure path should emit exactly 3 log lines")
	require.Contains(t, lines[0], "level=INFO")
	require.Contains(t, lines[1], "level=INFO")
	require.Contains(t, lines[2], "level=ERROR")
	require.Contains(t, lines[2], `msg="can't connect grpc server:"`)
}

func TestSyntheticGRPCProberEmitLogsHealthCheckFailed(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.GRPCSample{
		At:                  at,
		Success:             false,
		GRPCDuration:        0.05,
		HealthCheckResponse: 2, // NOT_SERVING
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticGRPCProber("example.com:5051")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "example.com:5051", registry, logger, "exec-1")
	require.False(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 3, "health-check-failure path should emit exactly 3 log lines")
	require.Contains(t, lines[0], "level=INFO")
	require.Contains(t, lines[1], "level=INFO")
	require.Contains(t, lines[2], "level=ERROR")
	// Production logs the exact same generic message for a clean
	// NOT_SERVING response as for an actual connection failure -- the
	// serving status is never logged, only exposed via the
	// probe_grpc_healthcheck_response gauge label (see emitLogs' doc
	// comment).
	require.Contains(t, lines[2], `msg="can't connect grpc server:"`)
	require.Contains(t, lines[2], `err=<nil>`)
}

func TestSyntheticGRPCProberName(t *testing.T) {
	require.Equal(t, "grpc", backfill.NewSyntheticGRPCProber("example.com:5051").Name())
}

func TestSyntheticGRPCProberSetTypedIgnoresMismatchedType(t *testing.T) {
	prober := backfill.NewSyntheticGRPCProber("example.com:5051")
	prober.SetTyped(backfill.NewTypedHTTPSample(testSample(time.Now())))
	// Should not panic and should leave the gRPC sample zero-valued; Probe
	// still runs (Normalize fills defaults) without error.
	registry := prometheus.NewRegistry()
	logger := log.NewLogfmtLogger(io.Discard)
	success, _ := prober.Probe(context.Background(), "example.com:5051", registry, logger, "exec-1")
	require.False(t, success)
}
