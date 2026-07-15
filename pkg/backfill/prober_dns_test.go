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

func dnsCheck() sm.Check {
	check := testCheck().Check
	check.Settings = sm.CheckSettings{Dns: &sm.DnsSettings{}}
	check.Target = "8.8.8.8"
	return check
}

// TestSyntheticDNSProberFixtureSuperset is the objective acceptance gate: the
// series names CollectTyped emits for a successful DNS check must exactly
// match the series names declared by the oracle fixture
// internal/scraper/testdata/dns.txt.
func TestSyntheticDNSProberFixtureSuperset(t *testing.T) {
	ctx := context.Background()
	check := dnsCheck()
	probe := testProbe()

	gen, err := backfill.NewGeneratorForCheck(ctx, check, probe)
	require.NoError(t, err)

	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.NewTypedDNSSample(backfill.DNSSample{
		Success:        true,
		QuerySucceeded: true,
		AnswerRRS:      1,
		LookupSeconds:  0.000123792,
		ConnectSeconds: 0.000294207,
		RequestSeconds: 0.003278084,
	})

	ts, streams, err := gen.CollectTyped(ctx, at, sample)
	require.NoError(t, err)
	require.NotEmpty(t, ts)
	require.NotEmpty(t, streams)

	got := metricNames(ts)
	want := fixtureMetricNames(t, "../../internal/scraper/testdata/dns.txt")
	require.ElementsMatch(t, want, got)
}

func TestSyntheticDNSProberEmitLogsSuccess(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.DNSSample{
		At:             at,
		Success:        true,
		QuerySucceeded: true,
		AnswerRRS:      1,
		AuthorityRRS:   2,
		AdditionalRRS:  1,
		Rcode:          "NOERROR",
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticDNSProber("8.8.8.8")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "8.8.8.8", registry, logger, "exec-1")
	require.True(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 7, "success path should emit exactly 7 log lines (includes Authority/Additional RRs validation)")

	for _, line := range lines {
		require.Contains(t, line, "level=INFO")
	}

	joined := strings.Join(lines, "\n")
	require.Contains(t, joined, `msg="Resolving target address"`)
	require.Contains(t, joined, `msg="Resolved target address"`)
	require.Contains(t, joined, `msg="Making DNS query"`)
	require.Contains(t, joined, "type=A")
	require.Contains(t, joined, "class=IN")
	require.Contains(t, joined, `msg="Got response"`)
	require.Contains(t, joined, "rcode=NOERROR")
	require.Contains(t, joined, "answer_rrs=1")
	require.Contains(t, joined, `msg="Validating Answer RRs"`)
	require.Contains(t, joined, `msg="Validating Authority RRs"`)
	require.Contains(t, joined, "authority_rrs=2")
	require.Contains(t, joined, `msg="Validating Additional RRs"`)
	require.Contains(t, joined, "additional_rrs=1")
}

func TestSyntheticDNSProberEmitLogsQueryFailure(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.DNSSample{
		At:             at,
		Success:        false,
		QuerySucceeded: false,
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticDNSProber("8.8.8.8")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "8.8.8.8", registry, logger, "exec-1")
	require.False(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 4, "query-failure path should emit exactly 4 log lines")
	require.Contains(t, lines[0], "level=INFO")
	require.Contains(t, lines[1], "level=INFO")
	require.Contains(t, lines[2], "level=INFO")
	require.Contains(t, lines[3], "level=error")
	require.Contains(t, lines[3], `msg="Error while sending a DNS query"`)
}

func TestSyntheticDNSProberEmitLogsValidationFailure(t *testing.T) {
	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	sample := backfill.DNSSample{
		At:             at,
		Success:        false,
		QuerySucceeded: true,
		Rcode:          "NXDOMAIN",
	}

	var buf strings.Builder
	logger := log.NewLogfmtLogger(&buf)
	prober := backfill.NewSyntheticDNSProber("8.8.8.8")
	prober.SetSample(sample)

	registry := prometheus.NewRegistry()
	success, _ := prober.Probe(context.Background(), "8.8.8.8", registry, logger, "exec-1")
	require.False(t, success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 5, "rcode/validation-failure path should emit exactly 5 log lines")
	require.Contains(t, lines[0], "level=INFO")
	require.Contains(t, lines[1], "level=INFO")
	require.Contains(t, lines[2], "level=INFO")
	require.Contains(t, lines[3], "level=INFO")
	require.Contains(t, lines[4], "level=error")
	require.Contains(t, lines[4], `msg="Answer RRs validation failed"`)
	require.Contains(t, lines[4], "rcode=NXDOMAIN")
}

func TestSyntheticDNSProberName(t *testing.T) {
	require.Equal(t, "dns", backfill.NewSyntheticDNSProber("8.8.8.8").Name())
}

func TestSyntheticDNSProberSetTypedIgnoresMismatchedType(t *testing.T) {
	prober := backfill.NewSyntheticDNSProber("8.8.8.8")
	prober.SetTyped(backfill.NewTypedHTTPSample(testSample(time.Now())))
	// Should not panic and should leave the DNS sample zero-valued; Probe
	// still runs (Normalize fills defaults) without error.
	registry := prometheus.NewRegistry()
	logger := log.NewLogfmtLogger(io.Discard)
	success, _ := prober.Probe(context.Background(), "8.8.8.8", registry, logger, "exec-1")
	require.False(t, success)
}
