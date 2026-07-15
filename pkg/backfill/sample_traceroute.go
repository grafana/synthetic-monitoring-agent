package backfill

import (
	"fmt"
	"hash/fnv"
	"time"
)

// TracerouteHop describes one hop in a synthetic traceroute execution. It
// drives one per-hop log line (see SyntheticTracerouteProber.emitLogs); Host
// and Loss also feed RouteHash/PacketLossPercent derivation in Normalize.
type TracerouteHop struct {
	Host       string
	RTTSeconds float64
	Loss       bool
}

// TracerouteSample describes one synthetic traceroute probe execution,
// mirroring Sample's role for HTTP. TypedTracerouteSample (below) adapts it
// to TypedSample, mirroring TypedHTTPSample's role for Sample.
//
// Unlike the other check types, per-hop LOG lines are the primary deliverable
// here: the metrics surface is only three gauges (see registerMetrics), but
// the catalogue's diagnosis paths (e.g. "which hop introduced loss") read the
// per-hop lines emitted from Hops.
type TracerouteSample struct {
	At time.Time

	// Success is the overall check result: false if the traceroute run
	// itself failed to execute (mirrors internal/prober/traceroute: the real
	// check's Success reflects whether mtr.RunWithContext returned an error,
	// not whether individual hops saw packet loss).
	Success bool

	// DurationSeconds is the overall probe duration returned to the scraper.
	// Normalize defaults it to the sum of per-hop RTTSeconds when zero.
	DurationSeconds float64

	// TotalHops feeds probe_traceroute_total_hops. If Hops is supplied,
	// Normalize overwrites TotalHops with len(Hops) (Hops is the source of
	// truth); otherwise it's the hop count to synthesize (default 3).
	TotalHops int

	// RouteHash feeds probe_traceroute_route_hash. The real check computes
	// this as fnv.New32().Sum32() over the concatenation of per-hop hostnames
	// (internal/prober/traceroute/traceroute.go: hostsString/traceHash), so
	// that the same path always produces the same hash -- that determinism
	// is the semantic the metric exists to expose (route volatility
	// detection). Normalize derives it the same way from Hops when zero.
	RouteHash uint64

	// PacketLossPercent feeds probe_traceroute_packet_loss_percent. Despite
	// the "_percent" name, production sets this gauge to an UNSCALED 0-1
	// ratio -- internal/prober/traceroute/traceroute.go:165-166:
	//
	//	overallPacketLoss := totalPacketsLost / totalPacketsSent
	//	overallPacketLossGauge.Set(overallPacketLoss)
	//
	// (no `*100` anywhere in that function). That's a real upstream quirk --
	// the metric name promises a percentage but the value landing in the
	// gauge is a fraction -- and it's mirrored here (rather than "corrected"
	// to a true 0-100 percentage) since the synthetic sample's job is to
	// reproduce what a real check actually emits. Normalize derives it as a
	// 0-1 fraction from Hops with Loss=true when zero and Hops is non-empty.
	PacketLossPercent float64

	// Hops is the per-hop detail driving the per-hop log lines and the
	// RouteHash/PacketLossPercent/TotalHops derivations above. If empty,
	// Normalize synthesizes TotalHops plausible hops with ascending RTTs and
	// no loss.
	Hops []TracerouteHop
}

// Normalize fills sensible defaults for zero-valued fields, synthesizing
// hops when none are given.
func (s *TracerouteSample) Normalize() {
	if s.At.IsZero() {
		s.At = time.Now().UTC()
	}

	if len(s.Hops) == 0 {
		if s.TotalHops <= 0 {
			s.TotalHops = 3
		}
		s.Hops = make([]TracerouteHop, 0, s.TotalHops)
		for i := 1; i <= s.TotalHops; i++ {
			s.Hops = append(s.Hops, TracerouteHop{
				Host:       fmt.Sprintf("hop-%d.transit.example", i),
				RTTSeconds: float64(i) * 0.008,
			})
		}
	} else {
		s.TotalHops = len(s.Hops)
	}

	if s.RouteHash == 0 {
		s.RouteHash = uint64(tracerouteRouteHash(s.Hops))
	}

	if s.PacketLossPercent == 0 {
		lossy := 0
		for _, hop := range s.Hops {
			if hop.Loss {
				lossy++
			}
		}
		if lossy > 0 {
			// 0-1 fraction, matching production's unscaled ratio (see the
			// field doc comment above and traceroute.go:165-166).
			s.PacketLossPercent = float64(lossy) / float64(len(s.Hops))
		}
	}

	if s.DurationSeconds == 0 {
		for _, hop := range s.Hops {
			s.DurationSeconds += hop.RTTSeconds
		}
	}
}

// tracerouteRouteHash mirrors the real check's route-hash derivation
// (internal/prober/traceroute/traceroute.go: hostsString is the
// concatenation of each hop's (comma-joined) target hostnames in TTL order,
// then fnv.New32().Write/Sum32()). Same path -> same hash is the point.
func tracerouteRouteHash(hops []TracerouteHop) uint32 {
	h := fnv.New32()
	for _, hop := range hops {
		_, _ = h.Write([]byte(hop.Host))
	}
	return h.Sum32()
}

// TypedTracerouteSample adapts TracerouteSample to TypedSample so traceroute
// checks can be driven through the generic CollectTyped path, mirroring
// TypedHTTPSample.
type TypedTracerouteSample struct {
	Sample TracerouteSample
}

// NewTypedTracerouteSample wraps an existing TracerouteSample for use with
// CollectTyped.
func NewTypedTracerouteSample(sample TracerouteSample) TypedTracerouteSample {
	return TypedTracerouteSample{Sample: sample}
}

func (s TypedTracerouteSample) Timestamp() time.Time {
	return s.Sample.At
}

func (s TypedTracerouteSample) WithTimestamp(t time.Time) TypedSample {
	s.Sample.At = t
	return s
}

func (s TypedTracerouteSample) Normalize() TypedSample {
	s.Sample.Normalize()
	return s
}

func (s TypedTracerouteSample) Succeeded() bool {
	return s.Sample.Success
}

func (s TypedTracerouteSample) DurationSeconds() float64 {
	return s.Sample.DurationSeconds
}

var _ TypedSample = TypedTracerouteSample{}
