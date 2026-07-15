package backfill_test

import (
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/pkg/backfill"
	"github.com/stretchr/testify/require"
)

func TestTracerouteSampleNormalizeSynthesizesHops(t *testing.T) {
	sample := backfill.TracerouteSample{
		Success:   true,
		TotalHops: 4,
	}
	sample.Normalize()

	require.False(t, sample.At.IsZero())
	require.Len(t, sample.Hops, 4)
	require.Equal(t, 4, sample.TotalHops)

	// Ascending RTTs, plausible transit hostnames.
	var prevRTT float64
	for i, hop := range sample.Hops {
		require.Contains(t, hop.Host, "transit.example")
		require.Greater(t, hop.RTTSeconds, prevRTT)
		prevRTT = hop.RTTSeconds
		require.False(t, hop.Loss)
		_ = i
	}
	require.NotZero(t, sample.RouteHash)
	require.Zero(t, sample.PacketLossPercent)
}

func TestTracerouteSampleNormalizeDefaultHopCount(t *testing.T) {
	sample := backfill.TracerouteSample{Success: true}
	sample.Normalize()

	require.NotEmpty(t, sample.Hops)
	require.Equal(t, len(sample.Hops), sample.TotalHops)
}

func TestTracerouteSampleNormalizeRouteHashDeterministic(t *testing.T) {
	hops := []backfill.TracerouteHop{
		{Host: "10.0.0.1", RTTSeconds: 0.001},
		{Host: "10.0.0.2", RTTSeconds: 0.002},
	}

	first := backfill.TracerouteSample{Success: true, Hops: append([]backfill.TracerouteHop{}, hops...)}
	first.Normalize()

	second := backfill.TracerouteSample{Success: true, Hops: append([]backfill.TracerouteHop{}, hops...)}
	second.Normalize()

	require.Equal(t, first.RouteHash, second.RouteHash)
	require.NotZero(t, first.RouteHash)

	// A different path must (in practice) produce a different hash.
	different := backfill.TracerouteSample{
		Success: true,
		Hops: []backfill.TracerouteHop{
			{Host: "10.0.0.9", RTTSeconds: 0.001},
			{Host: "10.0.0.8", RTTSeconds: 0.002},
		},
	}
	different.Normalize()
	require.NotEqual(t, first.RouteHash, different.RouteHash)
}

func TestTracerouteSampleNormalizePacketLossFromHops(t *testing.T) {
	sample := backfill.TracerouteSample{
		Success: true,
		Hops: []backfill.TracerouteHop{
			{Host: "10.0.0.1", RTTSeconds: 0.001},
			{Host: "10.0.0.2", RTTSeconds: 0.002, Loss: true},
			{Host: "10.0.0.3", RTTSeconds: 0.003},
			{Host: "10.0.0.4", RTTSeconds: 0.004},
		},
	}
	sample.Normalize()

	// 0-1 fraction (production quirk: probe_traceroute_packet_loss_percent
	// is actually unscaled despite the name -- traceroute.go:165-166).
	require.InDelta(t, 0.25, sample.PacketLossPercent, 0.0001)
}

func TestTracerouteSampleNormalizePreservesExplicitValues(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.TracerouteSample{
		At:                at,
		Success:           true,
		DurationSeconds:   1.5,
		TotalHops:         2,
		RouteHash:         42,
		PacketLossPercent: 10,
		Hops: []backfill.TracerouteHop{
			{Host: "hop-1.example", RTTSeconds: 0.01},
			{Host: "hop-2.example", RTTSeconds: 0.02},
		},
	}
	sample.Normalize()

	require.Equal(t, at, sample.At)
	require.InDelta(t, 1.5, sample.DurationSeconds, 0.0001)
	require.Equal(t, 2, sample.TotalHops)
	require.Equal(t, uint64(42), sample.RouteHash)
	require.InDelta(t, 10.0, sample.PacketLossPercent, 0.0001)
}

func TestTypedTracerouteSampleImplementsTypedSample(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.NewTypedTracerouteSample(backfill.TracerouteSample{
		Success:         true,
		DurationSeconds: 0.05,
	})

	require.Equal(t, at, sample.WithTimestamp(at).Timestamp())
	require.True(t, sample.Succeeded())
	require.InDelta(t, 0.05, sample.DurationSeconds(), 0.0001)

	normalized := sample.Normalize()
	require.NotNil(t, normalized)
}
