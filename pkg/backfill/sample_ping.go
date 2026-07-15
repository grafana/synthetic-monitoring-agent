package backfill

import "time"

// PingSample describes one synthetic ICMP ping probe execution, mirroring
// Sample's role for HTTP. TypedPingSample (below) adapts it to TypedSample,
// mirroring TypedHTTPSample's role for Sample.
type PingSample struct {
	At time.Time

	// Success is the overall check result: false if DNS resolution failed,
	// packet sending/receiving failed, or insufficient packets were received.
	Success bool

	// DNSLookupSeconds is the time spent resolving the target address; it
	// feeds both probe_dns_lookup_time_seconds and the "resolve" phase of
	// probe_icmp_duration_seconds.
	DNSLookupSeconds float64

	// DurationSeconds is the overall probe duration returned to the scraper
	// (which derives probe_duration_seconds/probe_all_duration_seconds from it).
	DurationSeconds float64

	// RTT metrics: min, max, and standard deviation in seconds.
	RTTMin    float64
	RTTMax    float64
	RTTStddev float64

	// ICMPDuration is the duration of the ICMP request phase (RTT time).
	// This feeds the "rtt" phase of probe_icmp_duration_seconds.
	ICMPDuration float64

	// ReplyHopLimit is the TTL/hop-limit from the reply packet.
	ReplyHopLimit float64

	// PacketsSent and PacketsReceived track ICMP packet counts.
	PacketsSent     int64
	PacketsReceived int64

	IPProtocol float64
	IPAddrHash float64
}

// Normalize fills sensible defaults for zero-valued fields.
func (s *PingSample) Normalize() {
	if s.At.IsZero() {
		s.At = time.Now().UTC()
	}
	if s.IPProtocol == 0 {
		s.IPProtocol = 4
	}
	if s.IPAddrHash == 0 {
		s.IPAddrHash = 9.9635399e7
	}
	if s.ReplyHopLimit == 0 {
		s.ReplyHopLimit = 56
	}
	// PacketsSent always defaults to 3 regardless of outcome: the real ICMP
	// prober (internal/prober/icmp/icmp_impl.go) always attempts the
	// configured packet count, win or lose. PacketsReceived only defaults to
	// 3 on success -- a lost/failed execution's real received count is 0,
	// not 3 (a fully-lost probe reports 0% received, not a false 100%); an
	// explicit caller-supplied PacketsReceived (e.g. a partial-loss count)
	// is never overwritten either way.
	if s.PacketsSent == 0 {
		s.PacketsSent = 3
	}
	if s.PacketsReceived == 0 && s.Success {
		s.PacketsReceived = 3
	}
	if s.RTTMin == 0 && s.RTTMax == 0 && s.RTTStddev == 0 && s.ICMPDuration > 0 {
		s.RTTMin = s.ICMPDuration
		s.RTTMax = s.ICMPDuration
		s.RTTStddev = 0
	}

	// Enforce the physical invariants a real ICMP RTT distribution always
	// satisfies: min can't exceed max, and a standard deviation can't be
	// negative. Explicit callers that hand in an inconsistent pair (e.g. a
	// swapped min/max) get corrected rather than silently propagated into
	// probe_icmp_duration_rtt_{min,max}_seconds.
	if s.RTTMin > s.RTTMax {
		s.RTTMin, s.RTTMax = s.RTTMax, s.RTTMin
	}
	if s.RTTStddev < 0 {
		s.RTTStddev = -s.RTTStddev
	}
}

// TypedPingSample adapts PingSample to TypedSample so ICMP ping checks can be
// driven through the generic CollectTyped path, mirroring TypedHTTPSample.
type TypedPingSample struct {
	Sample PingSample
}

// NewTypedPingSample wraps an existing PingSample for use with CollectTyped.
func NewTypedPingSample(sample PingSample) TypedPingSample {
	return TypedPingSample{Sample: sample}
}

func (s TypedPingSample) Timestamp() time.Time {
	return s.Sample.At
}

func (s TypedPingSample) WithTimestamp(t time.Time) TypedSample {
	s.Sample.At = t
	return s
}

func (s TypedPingSample) Normalize() TypedSample {
	s.Sample.Normalize()
	return s
}

func (s TypedPingSample) Succeeded() bool {
	return s.Sample.Success
}

func (s TypedPingSample) DurationSeconds() float64 {
	return s.Sample.DurationSeconds
}

var _ TypedSample = TypedPingSample{}
