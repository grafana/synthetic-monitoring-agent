package backfill

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// tracerouteSentPackets mirrors the mtr Module's default packet count per hop
// (internal/prober/traceroute/traceroute.go: settingsToModule's count: 5).
// The synthetic sample has no per-hop sent/lost counters of its own (Hops
// only carries a Loss bool), so this constant stands in for "Sent" on every
// hop line.
const tracerouteSentPackets = 5

// SyntheticTracerouteProber registers the three probe_traceroute_* gauges
// (+envelope) from a configured TracerouteSample, mirroring
// SyntheticHTTPProber's role for HTTP. Unlike the other synthetic probers,
// the per-hop LOG lines emitted by emitLogs are the primary deliverable: the
// catalogue's diagnosis paths (e.g. identifying a lossy hop) read those lines
// directly, since the metrics surface carries no per-hop detail.
type SyntheticTracerouteProber struct {
	target string
	sample TracerouteSample
}

func NewSyntheticTracerouteProber(target string) *SyntheticTracerouteProber {
	return &SyntheticTracerouteProber{target: target}
}

func (p *SyntheticTracerouteProber) SetSample(sample TracerouteSample) {
	sample.Normalize()
	p.sample = sample
}

// SetTyped implements SyntheticProber. s is expected to be a
// TypedTracerouteSample, which is the only TypedSample the registry
// constructs for CheckTypeTraceroute; any other concrete type is a no-op
// since it cannot describe a traceroute sample.
func (p *SyntheticTracerouteProber) SetTyped(s TypedSample) {
	if typed, ok := s.(TypedTracerouteSample); ok {
		p.SetSample(typed.Sample)
	}
}

var _ SyntheticProber = (*SyntheticTracerouteProber)(nil)

func (p *SyntheticTracerouteProber) Name() string {
	return "traceroute"
}

func (p *SyntheticTracerouteProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	sample := p.sample
	sample.Normalize()
	if target == "" {
		target = p.target
	}

	p.emitLogs(l, target, sample)
	p.registerMetrics(registry, sample)

	return sample.Success, sample.DurationSeconds
}

// emitLogs models the log vocabulary of the real traceroute prober
// (internal/prober/traceroute/traceroute.go). The per-hop line there is:
//
//	logger.Log("Level", "info", "Destination", m.Address, "Hosts", t,
//	    "TTL", hop.TTL, "ElapsedTime", avgElapsedTime, "LossPercent", hop.Loss(),
//	    "Sent", hop.Sent, "TracerouteID", tracerouteID)
//
// Notably that call has no "msg" key and capitalizes "Level" (unlike the
// lowercase "level"/"msg"/"time" convention the other synthetic probers use)
// -- both details are mirrored exactly here since the harness scenarios key
// off these exact logfmt fields to identify a lossy hop.
//
// The real check also has a single failure-path line when the traceroute run
// itself errors (m.RunWithContext):
//
//	logger.Log("Level", "error", "msg", err.Error())
//
// which IS mirrored (that one line has both "Level" and "msg"). That line
// precedes the per-hop lines below, since the real check logs per-hop
// results even after a run error (using whatever partial hop stats exist).
//
// The real check does NOT emit a trailing summary line -- the last per-hop
// line (TTL == the final hop) is the terminal anchor for "the traceroute
// finished" in a real Loki stream. An earlier version of this synthetic
// prober fabricated a trailing "Traceroute finished" line; it has been
// removed so the log stream matches production exactly (see
// TracerouteLogLineCount, which now returns just the per-hop count).
//
// Per-execution line count (documented for the harness manifest; see
// TracerouteLogLineCount):
//   - success:      hops     (per-hop lines only)
//   - run failure:  hops + 1 (leading error line + per-hop lines)
func (p *SyntheticTracerouteProber) emitLogs(l logger.Logger, target string, sample TracerouteSample) {
	if !sample.Success {
		_ = l.Log("Level", "error", "msg", "traceroute run failed")
	}

	id := tracerouteRunID(target, sample)
	for i, hop := range sample.Hops {
		ttl := i + 1
		elapsed := time.Duration(hop.RTTSeconds * float64(time.Second))
		// Per-hop LossPercent IS a true 0-100 percentage in production
		// (github.com/tonobo/mtr/pkg/hop.HopStatistic.Loss(): Lost/Sent*100)
		// -- unlike the overall probe_traceroute_packet_loss_percent gauge,
		// which is unscaled (see TracerouteSample.PacketLossPercent's doc
		// comment). Only the overall gauge carries that quirk.
		lossPercent := 0.0
		if hop.Loss {
			lossPercent = 100.0
		}
		_ = l.Log(
			"Level", "info",
			"Destination", target,
			"Hosts", hop.Host,
			"TTL", ttl,
			"ElapsedTime", elapsed,
			"LossPercent", lossPercent,
			"Sent", tracerouteSentPackets,
			"TracerouteID", id,
		)
	}
}

// tracerouteRunID derives a deterministic per-run identifier from the target,
// timestamp, and hop hosts, standing in for the real check's uuid.New() (a
// random UUID minted once per Probe() call and shared across that run's hop
// lines -- see traceroute.go's tracerouteID). Determinism here (rather than
// real randomness) keeps CollectTyped's output reproducible for the same
// sample, matching the reproducibility the rest of the backfill package
// relies on.
func tracerouteRunID(target string, sample TracerouteSample) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(target))
	_, _ = h.Write([]byte(sample.At.Format(time.RFC3339Nano)))
	for _, hop := range sample.Hops {
		_, _ = h.Write([]byte(hop.Host))
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// TracerouteLogLineCount returns the number of log lines a *successful*
// traceroute execution with the given number of hops emits: exactly one line
// per hop (production emits no trailing summary line -- see emitLogs' doc
// comment). On a run failure (TracerouteSample.Success == false) one
// additional leading error line is emitted, i.e. the line count is
// TracerouteLogLineCount(hops)+1; see emitLogs' doc comment.
func TracerouteLogLineCount(hops int) int {
	return hops
}

func (p *SyntheticTracerouteProber) registerMetrics(registry *prometheus.Registry, sample TracerouteSample) {
	gauges := []struct {
		name  string
		help  string
		value float64
	}{
		{"probe_traceroute_total_hops", "Total hops to reach a traceroute destination", float64(sample.TotalHops)},
		{"probe_traceroute_route_hash", "Hash of all the hosts in a traceroute path. Used to determine route volatility.", float64(sample.RouteHash)},
		{"probe_traceroute_packet_loss_percent", "Overall percentage of packet loss during the traceroute", sample.PacketLossPercent},
	}

	for _, g := range gauges {
		gauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: g.name,
			Help: g.help,
		})
		registry.MustRegister(gauge)
		gauge.Set(g.value)
	}
}
