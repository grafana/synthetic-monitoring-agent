package backfill_test

import (
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/pkg/backfill"
	"github.com/stretchr/testify/require"
)

func TestTCPSampleNormalizeDefaults(t *testing.T) {
	sample := backfill.TCPSample{
		Success:         true,
		DurationSeconds: 0.05,
	}
	sample.Normalize()

	require.False(t, sample.At.IsZero())
	require.Equal(t, 4.0, sample.IPProtocol)
	require.NotZero(t, sample.IPAddrHash)
}

func TestTCPSampleNormalizePreservesExplicitValues(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.TCPSample{
		At:               at,
		Success:          true,
		DurationSeconds:  0.05,
		DNSLookupSeconds: 0.001,
		IPProtocol:       6,
		IPAddrHash:       123,
	}
	sample.Normalize()

	require.Equal(t, at, sample.At)
	require.Equal(t, 6.0, sample.IPProtocol)
	require.Equal(t, 123.0, sample.IPAddrHash)
}

func TestTypedTCPSampleImplementsTypedSample(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.NewTypedTCPSample(backfill.TCPSample{
		Success:         true,
		DurationSeconds: 0.05,
	})

	require.True(t, sample.WithTimestamp(at).Timestamp().IsZero() == false)
	require.Equal(t, at, sample.WithTimestamp(at).Timestamp())
	require.True(t, sample.Succeeded())
	require.InDelta(t, 0.05, sample.DurationSeconds(), 0.0001)

	normalized := sample.Normalize()
	require.NotNil(t, normalized)
}
