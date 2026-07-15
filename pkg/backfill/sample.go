package backfill

import "time"

// Sample describes one synthetic HTTP probe execution.
type Sample struct {
	At                 time.Time
	Success            bool
	StatusCode         int
	DurationSeconds    float64
	DNSLookupSeconds   float64
	ResolveSeconds     float64
	ConnectSeconds     float64
	TLSSeconds         float64
	ProcessingSeconds  float64
	TransferSeconds    float64
	HTTPVersion        float64
	SSL                bool
	IPProtocol         float64
	ContentLength      float64
	UncompressedLength float64
	Redirects          float64
	FailedDueToRegex   float64
	IPAddrHash         float64

	// SSL certificate/TLS fields (only emitted when SSL is true), mirroring
	// TCPSample's identically-named fields — the real bbe HTTP prober
	// (prober/http.go) registers exactly these plus probe_tls_cipher_info
	// (TCP/gRPC never emit a cipher gauge) when the connection is SSL.
	SSLEarliestCertExpiry              float64
	SSLLastChainExpiryTimestampSeconds float64
	SSLLastChainFingerprint            string
	SSLLastChainIssuer                 string
	SSLLastChainSerialNumber           string
	SSLLastChainSubject                string
	SSLLastChainSubjectAlternative     string
	TLSVersion                         string
	TLSCipher                          string
}

// Normalize fills zero-valued phase timings from DurationSeconds.
func (s *Sample) Normalize() {
	if s.At.IsZero() {
		s.At = time.Now().UTC()
	}
	if s.StatusCode == 0 {
		if s.Success {
			s.StatusCode = 200
		} else {
			s.StatusCode = 500
		}
	}
	if s.HTTPVersion == 0 {
		s.HTTPVersion = 1.1
	}
	if s.IPProtocol == 0 {
		s.IPProtocol = 4
	}
	if s.DNSLookupSeconds == 0 {
		s.DNSLookupSeconds = s.ResolveSeconds
	}
	if s.ResolveSeconds == 0 {
		s.ResolveSeconds = s.DNSLookupSeconds
	}
	if s.DurationSeconds == 0 {
		s.DurationSeconds = s.ResolveSeconds + s.ConnectSeconds + s.TLSSeconds + s.ProcessingSeconds + s.TransferSeconds
	}
	if s.ProcessingSeconds == 0 && s.DurationSeconds > 0 {
		fixed := s.ResolveSeconds + s.ConnectSeconds + s.TLSSeconds + s.TransferSeconds
		if fixed < s.DurationSeconds {
			s.ProcessingSeconds = s.DurationSeconds - fixed
		}
	}
	if s.IPAddrHash == 0 {
		s.IPAddrHash = 3.668918509e9
	}
}

// TypedSample is the check-type-agnostic sample interface consumed by
// Generator.CollectTyped. Each supported sm.CheckType has its own
// implementation (e.g. TypedHTTPSample wraps Sample for CheckTypeHttp);
// A2-A6 add the remaining ones alongside their prober constructors.
type TypedSample interface {
	Timestamp() time.Time
	WithTimestamp(time.Time) TypedSample
	Normalize() TypedSample
	Succeeded() bool
	DurationSeconds() float64
}

// TypedHTTPSample adapts Sample to TypedSample so HTTP checks can be driven
// through the generic CollectTyped path without changing Sample/Collect.
type TypedHTTPSample struct {
	Sample Sample
}

// NewTypedHTTPSample wraps an existing Sample for use with CollectTyped.
func NewTypedHTTPSample(sample Sample) TypedHTTPSample {
	return TypedHTTPSample{Sample: sample}
}

func (s TypedHTTPSample) Timestamp() time.Time {
	return s.Sample.At
}

func (s TypedHTTPSample) WithTimestamp(t time.Time) TypedSample {
	s.Sample.At = t
	return s
}

func (s TypedHTTPSample) Normalize() TypedSample {
	s.Sample.Normalize()
	return s
}

func (s TypedHTTPSample) Succeeded() bool {
	return s.Sample.Success
}

func (s TypedHTTPSample) DurationSeconds() float64 {
	return s.Sample.DurationSeconds
}

var _ TypedSample = TypedHTTPSample{}
