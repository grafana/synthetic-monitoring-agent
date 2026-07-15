package backfill_test

import (
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/pkg/backfill"
	"github.com/stretchr/testify/require"
)

func TestDNSSampleNormalizeDefaults(t *testing.T) {
	sample := backfill.DNSSample{
		Success:        true,
		QuerySucceeded: true,
		LookupSeconds:  0.001,
	}
	sample.Normalize()

	require.False(t, sample.At.IsZero())
	require.Equal(t, "NOERROR", sample.Rcode)
	require.Equal(t, uint32(1), sample.Serial)
	require.Equal(t, 4.0, sample.IPProtocol)
	require.NotZero(t, sample.IPAddrHash)
	require.InDelta(t, 0.001, sample.DurationSeconds, 0.0001)
}

func TestDNSSampleNormalizePreservesExplicitValues(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.DNSSample{
		At:             at,
		Success:        true,
		QuerySucceeded: true,
		Rcode:          "NXDOMAIN",
		Serial:         42,
		IPProtocol:     6,
		IPAddrHash:     123,
		LookupSeconds:  0.001,
		ConnectSeconds: 0.002,
		RequestSeconds: 0.003,
	}
	sample.Normalize()

	require.Equal(t, at, sample.At)
	require.Equal(t, "NXDOMAIN", sample.Rcode)
	require.Equal(t, uint32(42), sample.Serial)
	require.Equal(t, 6.0, sample.IPProtocol)
	require.Equal(t, 123.0, sample.IPAddrHash)
	require.InDelta(t, 0.006, sample.DurationSeconds, 0.0001)
}

func TestTypedDNSSampleImplementsTypedSample(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.NewTypedDNSSample(backfill.DNSSample{
		Success:         true,
		QuerySucceeded:  true,
		DurationSeconds: 0.05,
	})

	require.True(t, sample.WithTimestamp(at).Timestamp().IsZero() == false)
	require.Equal(t, at, sample.WithTimestamp(at).Timestamp())
	require.True(t, sample.Succeeded())
	require.InDelta(t, 0.05, sample.DurationSeconds(), 0.0001)

	normalized := sample.Normalize()
	require.NotNil(t, normalized)
}
