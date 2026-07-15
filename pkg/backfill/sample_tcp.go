package backfill

import "time"

// TCPSample describes one synthetic TCP probe execution, mirroring Sample's
// role for HTTP. TypedTCPSample (below) adapts it to TypedSample, mirroring
// TypedHTTPSample's role for Sample.
type TCPSample struct {
	At time.Time

	// Success is the overall check result: false if connection failed, dial
	// failed, timeout, send/expect mismatch, etc.
	Success bool

	// DNSLookupSeconds is the time spent resolving the target address; it
	// feeds probe_dns_lookup_time_seconds.
	DNSLookupSeconds float64

	// DurationSeconds is the overall probe duration returned to the scraper
	// (which derives probe_duration_seconds/probe_all_duration_seconds from
	// it).
	DurationSeconds float64

	// FailedDueToRegex reports whether the probe failed due to send/expect
	// mismatch. Feeds probe_failed_due_to_regex.
	FailedDueToRegex float64

	// SSL indicates whether this was an SSL/TLS connection. When true, SSL-related
	// fields below are emitted; when false, they're omitted.
	SSL bool

	// SSL certificate/TLS fields (only emitted when SSL is true)
	SSLEarliestCertExpiry              float64
	SSLLastChainExpiryTimestampSeconds float64
	SSLLastChainFingerprint            string
	SSLLastChainIssuer                 string
	SSLLastChainSerialNumber           string
	SSLLastChainSubject                string
	SSLLastChainSubjectAlternative     string
	TLSVersion                         string

	IPProtocol float64
	IPAddrHash float64
}

// Normalize fills sensible defaults for zero-valued fields.
func (s *TCPSample) Normalize() {
	if s.At.IsZero() {
		s.At = time.Now().UTC()
	}
	if s.IPProtocol == 0 {
		s.IPProtocol = 4
	}
	if s.IPAddrHash == 0 {
		s.IPAddrHash = 1.268118805e9
	}
}

// TypedTCPSample adapts TCPSample to TypedSample so TCP checks can be driven
// through the generic CollectTyped path, mirroring TypedHTTPSample.
type TypedTCPSample struct {
	Sample TCPSample
}

// NewTypedTCPSample wraps an existing TCPSample for use with CollectTyped.
func NewTypedTCPSample(sample TCPSample) TypedTCPSample {
	return TypedTCPSample{Sample: sample}
}

func (s TypedTCPSample) Timestamp() time.Time {
	return s.Sample.At
}

func (s TypedTCPSample) WithTimestamp(t time.Time) TypedSample {
	s.Sample.At = t
	return s
}

func (s TypedTCPSample) Normalize() TypedSample {
	s.Sample.Normalize()
	return s
}

func (s TypedTCPSample) Succeeded() bool {
	return s.Sample.Success
}

func (s TypedTCPSample) DurationSeconds() float64 {
	return s.Sample.DurationSeconds
}

var _ TypedSample = TypedTCPSample{}
