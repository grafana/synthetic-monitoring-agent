package backfill_test

import (
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/pkg/backfill"
	"github.com/stretchr/testify/require"
)

func TestPingSampleNormalizeDefaults(t *testing.T) {
	sample := backfill.PingSample{
		Success:      true,
		RTTMin:       0.00007,
		RTTMax:       0.00009,
		RTTStddev:    0.000008,
		ICMPDuration: 0.00008,
	}
	sample.Normalize()

	require.False(t, sample.At.IsZero())
	require.Equal(t, 4.0, sample.IPProtocol)
	require.NotZero(t, sample.IPAddrHash)
	require.Equal(t, 56.0, sample.ReplyHopLimit)
	require.Equal(t, int64(3), sample.PacketsSent)
	require.Equal(t, int64(3), sample.PacketsReceived)
}

// TestPingSampleNormalizeIPAddrHashMatchesFixture pins the M1 fix: ping's
// default IPAddrHash must match its own oracle fixture
// (internal/scraper/testdata/ping.txt: probe_ip_addr_hash 9.9635399e+07),
// not the dns/tcp/grpc/traceroute shared constant (1.268118805e9) that a
// prior copy-paste left here.
func TestPingSampleNormalizeIPAddrHashMatchesFixture(t *testing.T) {
	sample := backfill.PingSample{Success: true}
	sample.Normalize()

	require.InDelta(t, 9.9635399e7, sample.IPAddrHash, 1)
}

// TestPingSampleNormalizeGatesPacketCountsOnSuccess pins the I2 fix: a lost
// (failed) execution must default to PacketsSent=3 (the real ICMP prober --
// internal/prober/icmp/icmp_impl.go -- always attempts the configured packet
// count regardless of outcome) but PacketsReceived=0 (a fully-lost probe
// receives nothing back), not the pre-fix 3/3 which made every ping loss
// look like 0% packet loss.
func TestPingSampleNormalizeGatesPacketCountsOnSuccess(t *testing.T) {
	lost := backfill.PingSample{Success: false}
	lost.Normalize()

	require.Equal(t, int64(3), lost.PacketsSent)
	require.Equal(t, int64(0), lost.PacketsReceived)

	healthy := backfill.PingSample{Success: true}
	healthy.Normalize()

	require.Equal(t, int64(3), healthy.PacketsSent)
	require.Equal(t, int64(3), healthy.PacketsReceived)
}

// TestPingSampleNormalizePreservesExplicitPacketCounts ensures an explicit
// (e.g. partial-loss) PacketsReceived value on a failed execution is never
// overwritten by the success-gated default.
func TestPingSampleNormalizePreservesExplicitPacketCounts(t *testing.T) {
	sample := backfill.PingSample{Success: false, PacketsSent: 3, PacketsReceived: 1}
	sample.Normalize()

	require.Equal(t, int64(3), sample.PacketsSent)
	require.Equal(t, int64(1), sample.PacketsReceived)
}

func TestPingSampleNormalizePreservesExplicitValues(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.PingSample{
		At:               at,
		Success:          true,
		DNSLookupSeconds: 0.000005,
		DurationSeconds:  0.0001,
		RTTMin:           0.00007,
		RTTMax:           0.00009,
		RTTStddev:        0.000008,
		ICMPDuration:     0.00008,
		ReplyHopLimit:    64,
		IPProtocol:       6,
		IPAddrHash:       123,
	}
	sample.Normalize()

	require.Equal(t, at, sample.At)
	require.Equal(t, 6.0, sample.IPProtocol)
	require.Equal(t, 123.0, sample.IPAddrHash)
	require.Equal(t, 64.0, sample.ReplyHopLimit)
	require.InDelta(t, 0.00007, sample.RTTMin, 0.00000001)
	require.InDelta(t, 0.00009, sample.RTTMax, 0.00000001)
}

func TestPingSampleNormalizeFillsRTTFromICMPDuration(t *testing.T) {
	sample := backfill.PingSample{
		Success:      true,
		ICMPDuration: 0.0001,
	}
	sample.Normalize()

	require.InDelta(t, 0.0001, sample.RTTMin, 0.00000001)
	require.InDelta(t, 0.0001, sample.RTTMax, 0.00000001)
	require.Equal(t, 0.0, sample.RTTStddev)
}

func TestPingSampleNormalizeEnforcesRTTMinLessThanMax(t *testing.T) {
	sample := backfill.PingSample{
		Success: true,
		RTTMin:  0.0002, // deliberately swapped
		RTTMax:  0.0001,
	}
	sample.Normalize()

	require.InDelta(t, 0.0001, sample.RTTMin, 0.00000001)
	require.InDelta(t, 0.0002, sample.RTTMax, 0.00000001)
}

func TestPingSampleNormalizeEnforcesNonNegativeStddev(t *testing.T) {
	sample := backfill.PingSample{
		Success:   true,
		RTTMin:    0.0001,
		RTTMax:    0.0002,
		RTTStddev: -0.00005,
	}
	sample.Normalize()

	require.InDelta(t, 0.00005, sample.RTTStddev, 0.00000001)
}

func TestTypedPingSampleImplementsTypedSample(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := backfill.NewTypedPingSample(backfill.PingSample{
		Success:         true,
		DurationSeconds: 0.0001,
		RTTMin:          0.00007,
		RTTMax:          0.00009,
	})

	require.True(t, sample.WithTimestamp(at).Timestamp().IsZero() == false)
	require.Equal(t, at, sample.WithTimestamp(at).Timestamp())
	require.True(t, sample.Succeeded())
	require.InDelta(t, 0.0001, sample.DurationSeconds(), 0.00001)

	normalized := sample.Normalize()
	require.NotNil(t, normalized)
}
