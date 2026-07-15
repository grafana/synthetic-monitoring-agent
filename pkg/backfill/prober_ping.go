package backfill

import (
	"context"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// SyntheticPingProber registers probe_icmp_*/probe_dns_*/probe_ip_* metrics
// from a configured PingSample, mirroring SyntheticHTTPProber's role for HTTP.
type SyntheticPingProber struct {
	target string
	sample PingSample
}

func NewSyntheticPingProber(target string) *SyntheticPingProber {
	return &SyntheticPingProber{target: target}
}

func (p *SyntheticPingProber) SetSample(sample PingSample) {
	sample.Normalize()
	p.sample = sample
}

// SetTyped implements SyntheticProber. s is expected to be a TypedPingSample,
// which is the only TypedSample the registry constructs for CheckTypePing; any
// other concrete type is a no-op since it cannot describe a ping sample.
func (p *SyntheticPingProber) SetTyped(s TypedSample) {
	if typed, ok := s.(TypedPingSample); ok {
		p.SetSample(typed.Sample)
	}
}

var _ SyntheticProber = (*SyntheticPingProber)(nil)

func (p *SyntheticPingProber) Name() string {
	return "ping"
}

func (p *SyntheticPingProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	sample := p.sample
	sample.Normalize()
	if target == "" {
		target = p.target
	}

	p.emitLogs(l, target, sample)
	p.registerMetrics(registry, sample)

	return sample.Success, sample.DurationSeconds
}

// emitLogs models the log vocabulary of the real ICMP prober
// (internal/prober/icmp/icmp_impl.go): creating socket, using source address,
// creating packets, receiving replies, and getting probe statistics.
//
// Per-execution line count (documented for the harness manifest):
//   - success (3+ replies): 5 INFO lines (create socket, use source, create packet,
//     waiting for replies, probe finished with stats)
//   - loss/timeout (0 replies): 5 INFO lines (create socket, use source, create
//     packet, waiting for replies, "failed to run ping")
//
// Note: "failed to run ping" is logged at INFO, not ERROR, in production
// (internal/prober/icmp/icmp_impl.go:149: `level.Info(logger).Log("msg",
// "failed to run ping", "err", err.Error())`) -- a real quirk (a failed probe
// logged at an ostensibly non-error level) that's mirrored here rather than
// "corrected" to ERROR.
func (p *SyntheticPingProber) emitLogs(l logger.Logger, target string, sample PingSample) {
	times := []time.Time{
		sample.At,
		sample.At.Add(1 * time.Millisecond),
		sample.At.Add(2 * time.Millisecond),
		sample.At.Add(3 * time.Millisecond),
		sample.At.Add(4 * time.Millisecond),
	}
	logAt := func(level string, at time.Time, msg string, extra ...any) {
		attrs := []any{"level", level, "msg", msg, "time", at.Format(time.RFC3339Nano)}
		attrs = append(attrs, extra...)
		_ = l.Log(attrs...)
	}
	logInfo := func(at time.Time, msg string, extra ...any) { logAt("INFO", at, msg, extra...) }

	logInfo(times[0], "Creating socket")
	logInfo(times[1], "Using source address", "srcIP", "127.0.0.1")
	logInfo(times[2], "Creating ICMP packet", "seq", "1")
	logInfo(times[3], "Waiting for reply packets")

	if !sample.Success {
		logInfo(times[4], "failed to run ping", "err", "timeout")
		return
	}

	logInfo(times[4], "Probe finished", "packets_sent", "3", "packets_received", "3", "rtt_min", "71.084µs", "rtt_max", "91.042µs")
}

func (p *SyntheticPingProber) registerMetrics(registry *prometheus.Registry, sample PingSample) {
	gauges := []struct {
		name  string
		help  string
		value float64
	}{
		{"probe_dns_lookup_time_seconds", "Returns the time taken for probe dns lookup in seconds", sample.DNSLookupSeconds},
		{"probe_icmp_duration_rtt_min_seconds", "Minimum duration of round trip time phase", sample.RTTMin},
		{"probe_icmp_duration_rtt_max_seconds", "Maximum duration of round trip time phase", sample.RTTMax},
		{"probe_icmp_duration_rtt_stddev_seconds", "Standard deviation of round trip time phase", sample.RTTStddev},
		{"probe_icmp_packets_sent_count", "Number of ICMP packets sent", float64(sample.PacketsSent)},
		{"probe_icmp_packets_received_count", "Number of ICMP packets received", float64(sample.PacketsReceived)},
		{"probe_icmp_reply_hop_limit", "Replied packet hop limit (TTL for ipv4)", sample.ReplyHopLimit},
		{"probe_ip_protocol", "Specifies whether probe ip protocol is IP4 or IP6", sample.IPProtocol},
		{"probe_ip_addr_hash", "Specifies the hash of IP address. It's useful to detect if the IP address changes.", sample.IPAddrHash},
	}

	for _, g := range gauges {
		gauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: g.name,
			Help: g.help,
		})
		registry.MustRegister(gauge)
		gauge.Set(g.value)
	}

	// ICMP duration phases: resolve, setup, rtt
	phases := map[string]float64{
		"resolve": sample.DNSLookupSeconds,
		"setup":   0, // Not exposed in the sample; using 0 for now
		"rtt":     sample.ICMPDuration,
	}
	durationVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_icmp_duration_seconds",
		Help: "Duration of icmp request by phase",
	}, []string{"phase"})
	registry.MustRegister(durationVec)
	for phase, value := range phases {
		durationVec.WithLabelValues(phase).Set(value)
	}
}
