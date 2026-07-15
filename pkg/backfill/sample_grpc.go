package backfill

import "time"

// GRPCSample describes one synthetic gRPC probe execution, mirroring Sample's
// role for HTTP. TypedGRPCSample (below) adapts it to TypedSample, mirroring
// TypedHTTPSample's role for Sample.
type GRPCSample struct {
	At time.Time

	// Success is the overall check result: false if connection failed, dial
	// failed, timeout, health check failed, etc.
	Success bool

	// DNSLookupSeconds is the time spent resolving the target address; it
	// feeds probe_dns_lookup_time_seconds.
	DNSLookupSeconds float64

	// DurationSeconds is the overall probe duration returned to the scraper
	// (which derives probe_duration_seconds/probe_all_duration_seconds from
	// it).
	DurationSeconds float64

	// GRPCDuration is the time spent on the gRPC health check request (the
	// "check" phase). Feeds probe_grpc_duration_seconds{phase="check"}.
	GRPCDuration float64

	// StatusCode is the gRPC status code (e.g., 0 for OK). Feeds
	// probe_grpc_status_code.
	StatusCode int

	// ConnectFailed distinguishes the two distinct failure modes production
	// collapses into the same log line (see SyntheticGRPCProber.emitLogs):
	// true means the connection/RPC itself never completed (mirrors
	// blackbox_exporter/prober/grpc.go's `ok, _, _, servingStatus, err :=
	// client.Check(...)` returning a non-nil err, so servingStatus == "" and
	// no probe_grpc_healthcheck_response label is set); false (with
	// Success == false) means the RPC completed cleanly but reported a
	// non-SERVING status. This replaces the previous "GRPCDuration == 0"
	// discriminator, which conflated "connection never happened" with
	// "GRPCDuration wasn't supplied by the test/caller".
	ConnectFailed bool

	// HealthCheckResponse is the gRPC health check response status code
	// (SERVING=1 per the brief). Feeds probe_grpc_healthcheck_response
	// with labels matching the serving_status value.
	HealthCheckResponse int

	// SSL indicates whether this was an SSL/TLS connection. When true, SSL-related
	// fields below are emitted; when false, they're omitted.
	SSL bool

	// SSL certificate/TLS fields (only emitted when SSL is true)
	SSLEarliestCertExpiry          float64
	SSLLastChainFingerprint        string
	SSLLastChainIssuer             string
	SSLLastChainSerialNumber       string
	SSLLastChainSubject            string
	SSLLastChainSubjectAlternative string
	TLSVersion                     string

	IPProtocol float64
	IPAddrHash float64
}

// Normalize fills sensible defaults for zero-valued fields.
func (s *GRPCSample) Normalize() {
	if s.At.IsZero() {
		s.At = time.Now().UTC()
	}
	if s.IPProtocol == 0 {
		s.IPProtocol = 4
	}
	if s.IPAddrHash == 0 {
		s.IPAddrHash = 1.268118805e9
	}
	if s.HealthCheckResponse == 0 {
		switch {
		case s.Success:
			s.HealthCheckResponse = 1 // SERVING
		case s.ConnectFailed:
			// Production leaves servingStatus == "" when the Check RPC
			// itself errors (blackbox_exporter/prober/grpc.go:196: `if
			// servingStatus != "" { healthCheckResponseGaugeVec...Set(1) }`),
			// so no probe_grpc_healthcheck_response label is ever set to 1.
			// -1 is an out-of-range sentinel: registerMetrics's `i ==
			// sample.HealthCheckResponse` never matches any of the four
			// known statuses, so every label stays at its base value of 0,
			// matching production exactly.
			s.HealthCheckResponse = -1
		default:
			// A clean (non-connect-failure) failed health check: the RPC
			// completed and returned a non-SERVING status, so production
			// sets exactly that status's label to 1 (grpc.go:196-198).
			// NOT_SERVING is the representative "clean failure" status.
			s.HealthCheckResponse = 2 // NOT_SERVING
		}
	}
	if s.DurationSeconds == 0 && s.DNSLookupSeconds > 0 && s.GRPCDuration > 0 {
		s.DurationSeconds = s.DNSLookupSeconds + s.GRPCDuration
	}
}

// TypedGRPCSample adapts GRPCSample to TypedSample so gRPC checks can be driven
// through the generic CollectTyped path, mirroring TypedHTTPSample.
type TypedGRPCSample struct {
	Sample GRPCSample
}

// NewTypedGRPCSample wraps an existing GRPCSample for use with CollectTyped.
func NewTypedGRPCSample(sample GRPCSample) TypedGRPCSample {
	return TypedGRPCSample{Sample: sample}
}

func (s TypedGRPCSample) Timestamp() time.Time {
	return s.Sample.At
}

func (s TypedGRPCSample) WithTimestamp(t time.Time) TypedSample {
	s.Sample.At = t
	return s
}

func (s TypedGRPCSample) Normalize() TypedSample {
	s.Sample.Normalize()
	return s
}

func (s TypedGRPCSample) Succeeded() bool {
	return s.Sample.Success
}

func (s TypedGRPCSample) DurationSeconds() float64 {
	return s.Sample.DurationSeconds
}

var _ TypedSample = TypedGRPCSample{}
