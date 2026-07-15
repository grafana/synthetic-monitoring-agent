package backfill_test

import (
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/pkg/backfill"
	"github.com/stretchr/testify/require"
)

func TestGRPCSampleNormalizeDefaults(t *testing.T) {
	sample := backfill.GRPCSample{
		Success:          true,
		DNSLookupSeconds: 0.0001,
		GRPCDuration:     0.05,
	}
	sample.Normalize()

	require.False(t, sample.At.IsZero())
	require.Equal(t, 4.0, sample.IPProtocol)
	require.NotZero(t, sample.IPAddrHash)
	require.Equal(t, 1, sample.HealthCheckResponse) // SERVING
}

func TestGRPCSampleNormalizePreservesExplicitValues(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.GRPCSample{
		At:                  at,
		Success:             true,
		DNSLookupSeconds:    0.0001,
		GRPCDuration:        0.05,
		StatusCode:          0,
		HealthCheckResponse: 1,
		IPProtocol:          6,
		IPAddrHash:          123,
	}
	sample.Normalize()

	require.Equal(t, at, sample.At)
	require.Equal(t, 6.0, sample.IPProtocol)
	require.Equal(t, 123.0, sample.IPAddrHash)
	require.Equal(t, 1, sample.HealthCheckResponse)
}

func TestGRPCSampleNormalizeDefaultsNotServingWhenFailedCleanly(t *testing.T) {
	// A failed health check that wasn't a connection failure should default
	// to NOT_SERVING(2), matching production's single-label-set encoding for
	// a completed-but-non-serving response (blackbox_exporter/prober/grpc.go
	// :196-198). See sample_grpc.go's Normalize doc comment.
	sample := backfill.GRPCSample{
		Success: false,
	}
	sample.Normalize()

	require.Equal(t, 2, sample.HealthCheckResponse) // NOT_SERVING
}

func TestGRPCSampleNormalizeSentinelHealthCheckResponseWhenConnectFailed(t *testing.T) {
	// A real connection failure leaves servingStatus == "" in production, so
	// no probe_grpc_healthcheck_response label is ever set to 1. -1 is an
	// out-of-range sentinel that reproduces that (registerMetrics never
	// matches it against any of the four known statuses).
	sample := backfill.GRPCSample{
		Success:       false,
		ConnectFailed: true,
	}
	sample.Normalize()

	require.Equal(t, -1, sample.HealthCheckResponse)
}

func TestGRPCSampleNormalizeExplicitHealthCheckResponseWins(t *testing.T) {
	sample := backfill.GRPCSample{
		Success:             false,
		ConnectFailed:       true,
		HealthCheckResponse: 3, // SERVICE_UNKNOWN, explicit
	}
	sample.Normalize()

	require.Equal(t, 3, sample.HealthCheckResponse)
}

func TestGRPCSampleNormalizeDurationSeconds(t *testing.T) {
	sample := backfill.GRPCSample{
		Success:          true,
		DNSLookupSeconds: 0.0001,
		GRPCDuration:     0.05,
	}
	sample.Normalize()

	require.InDelta(t, 0.0501, sample.DurationSeconds, 0.0001)
}

func TestTypedGRPCSampleImplementsTypedSample(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.NewTypedGRPCSample(backfill.GRPCSample{
		Success:          true,
		DNSLookupSeconds: 0.0001,
		GRPCDuration:     0.05,
	})

	require.True(t, sample.WithTimestamp(at).Timestamp().IsZero() == false)
	require.Equal(t, at, sample.WithTimestamp(at).Timestamp())
	require.True(t, sample.Succeeded())

	normalized := sample.Normalize()
	require.NotNil(t, normalized)
	require.InDelta(t, 0.0501, normalized.DurationSeconds(), 0.0001)
}
