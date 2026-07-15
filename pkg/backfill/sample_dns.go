package backfill

import "time"

// DNSSample describes one synthetic DNS probe execution, mirroring Sample's
// role for HTTP. TypedDNSSample (below) adapts it to TypedSample, mirroring
// TypedHTTPSample's role for Sample.
type DNSSample struct {
	At time.Time

	// Success is the overall check result: false if resolution failed, the
	// query itself failed, the rcode wasn't in the valid list, or RRset
	// validation failed.
	Success bool

	// LookupSeconds is the time spent resolving the DNS server address; it
	// feeds both probe_dns_lookup_time_seconds and the "resolve" phase of
	// probe_dns_duration_seconds.
	LookupSeconds float64
	// ConnectSeconds and RequestSeconds are the "connect"/"request" phases of
	// probe_dns_duration_seconds.
	ConnectSeconds float64
	RequestSeconds float64
	// DurationSeconds is the overall probe duration returned to the scraper
	// (which derives probe_duration_seconds/probe_all_duration_seconds from
	// it). Defaults to the sum of the phase timings above.
	DurationSeconds float64

	AnswerRRS     int
	AuthorityRRS  int
	AdditionalRRS int

	// QuerySucceeded reports whether a response was received at all (as
	// opposed to Success, which also requires a valid rcode and RRset
	// validation). It feeds probe_dns_query_succeeded.
	QuerySucceeded bool

	// Serial is the SOA zone serial. The fixture always emits
	// probe_dns_serial regardless of query type, so it's unconditional here
	// (unlike the real prober, which only registers it for TypeSOA queries).
	Serial uint32

	// Rcode is log-only (e.g. "NOERROR", "NXDOMAIN"); it has no dedicated
	// gauge of its own.
	Rcode string

	IPProtocol float64
	IPAddrHash float64
}

// Normalize fills sensible defaults for zero-valued fields.
func (s *DNSSample) Normalize() {
	if s.At.IsZero() {
		s.At = time.Now().UTC()
	}
	if s.Rcode == "" {
		s.Rcode = "NOERROR"
	}
	if s.Serial == 0 {
		s.Serial = 1
	}
	if s.IPProtocol == 0 {
		s.IPProtocol = 4
	}
	if s.IPAddrHash == 0 {
		s.IPAddrHash = 1.268118805e9
	}
	if s.DurationSeconds == 0 {
		s.DurationSeconds = s.LookupSeconds + s.ConnectSeconds + s.RequestSeconds
	}
	if s.LookupSeconds == 0 && s.ConnectSeconds == 0 && s.RequestSeconds == 0 && s.DurationSeconds > 0 {
		s.LookupSeconds = s.DurationSeconds
	}
}

// TypedDNSSample adapts DNSSample to TypedSample so DNS checks can be driven
// through the generic CollectTyped path, mirroring TypedHTTPSample.
type TypedDNSSample struct {
	Sample DNSSample
}

// NewTypedDNSSample wraps an existing DNSSample for use with CollectTyped.
func NewTypedDNSSample(sample DNSSample) TypedDNSSample {
	return TypedDNSSample{Sample: sample}
}

func (s TypedDNSSample) Timestamp() time.Time {
	return s.Sample.At
}

func (s TypedDNSSample) WithTimestamp(t time.Time) TypedSample {
	s.Sample.At = t
	return s
}

func (s TypedDNSSample) Normalize() TypedSample {
	s.Sample.Normalize()
	return s
}

func (s TypedDNSSample) Succeeded() bool {
	return s.Sample.Success
}

func (s TypedDNSSample) DurationSeconds() float64 {
	return s.Sample.DurationSeconds
}

var _ TypedSample = TypedDNSSample{}
