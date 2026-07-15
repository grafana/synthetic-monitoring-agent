package backfill

import (
	"context"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// SyntheticProber is the check-type-agnostic prober interface used by
// Generator.CollectTyped. Every constructor registered in proberConstructors
// (see generator.go) must return a value satisfying this interface.
type SyntheticProber interface {
	prober.Prober
	SetTyped(TypedSample)
}

const goZeroTimestamp = "0001-01-01T00:00:00Z"

// SyntheticHTTPProber registers probe_* metrics from a configured Sample.
type SyntheticHTTPProber struct {
	target string
	sample Sample
}

func NewSyntheticHTTPProber(target string) *SyntheticHTTPProber {
	return &SyntheticHTTPProber{target: target}
}

func (p *SyntheticHTTPProber) SetSample(sample Sample) {
	sample.Normalize()
	p.sample = sample
}

// SetTyped implements SyntheticProber. s is expected to be a TypedHTTPSample,
// which is the only TypedSample the registry constructs for CheckTypeHttp; any
// other concrete type is a no-op since it cannot describe an HTTP sample.
func (p *SyntheticHTTPProber) SetTyped(s TypedSample) {
	if typed, ok := s.(TypedHTTPSample); ok {
		p.SetSample(typed.Sample)
	}
}

var _ SyntheticProber = (*SyntheticHTTPProber)(nil)

func (p *SyntheticHTTPProber) Name() string {
	return "http"
}

func (p *SyntheticHTTPProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	sample := p.sample
	sample.Normalize()
	if target == "" {
		target = p.target
	}

	p.emitLogs(l, target, sample)
	p.registerMetrics(registry, sample)

	return sample.Success, sample.DurationSeconds
}

func (p *SyntheticHTTPProber) emitLogs(l logger.Logger, target string, sample Sample) {
	times := []time.Time{
		sample.At,
		sample.At.Add(1 * time.Millisecond),
		sample.At.Add(2 * time.Millisecond),
		sample.At.Add(3 * time.Millisecond),
		sample.At.Add(4 * time.Millisecond),
	}
	logInfo := func(at time.Time, msg string, extra ...any) {
		attrs := []any{"level", "INFO", "msg", msg, "time", at.Format(time.RFC3339Nano)}
		attrs = append(attrs, extra...)
		_ = l.Log(attrs...)
	}

	logInfo(times[0], "Resolving target address", "target", target)
	logInfo(times[1], "Resolved target address", "target", target, "ip", "127.0.0.1")
	logInfo(times[2], "Making HTTP request", "url", target, "host", target)
	logInfo(times[3], "Received HTTP response", "status_code", sample.StatusCode)

	trace := buildRoundTripTimestamps(sample)
	logInfo(times[4], "Response timings for roundtrip",
		"roundtrip", "1",
		"start", formatBBETimestamp(trace.start),
		"dnsDone", formatBBETimestamp(trace.dnsDone),
		"connectDone", formatBBETimestamp(trace.connectDone),
		"gotConn", formatBBETimestamp(trace.gotConn),
		"responseStart", formatBBETimestamp(trace.responseStart),
		"tlsStart", formatBBETimestamp(trace.tlsStart),
		"tlsDone", formatBBETimestamp(trace.tlsDone),
		"end", formatBBETimestamp(trace.end),
	)
}

type roundTripTimestamps struct {
	start         time.Time
	dnsDone       time.Time
	connectDone   time.Time
	gotConn       time.Time
	responseStart time.Time
	tlsStart      time.Time
	tlsDone       time.Time
	end           time.Time
}

func buildRoundTripTimestamps(sample Sample) roundTripTimestamps {
	start := sample.At.UTC()
	dnsDone := start.Add(seconds(sample.ResolveSeconds))
	connectDone := dnsDone.Add(seconds(sample.ConnectSeconds))

	trace := roundTripTimestamps{
		start:       start,
		dnsDone:     dnsDone,
		connectDone: connectDone,
	}

	if sample.SSL && sample.TLSSeconds > 0 {
		trace.tlsStart = connectDone
		trace.tlsDone = trace.tlsStart.Add(seconds(sample.TLSSeconds))
		trace.gotConn = trace.tlsDone
	} else {
		trace.gotConn = connectDone
	}

	trace.responseStart = trace.gotConn.Add(seconds(sample.ProcessingSeconds))
	trace.end = trace.responseStart.Add(seconds(sample.TransferSeconds))
	return trace
}

func seconds(value float64) time.Duration {
	return time.Duration(value * float64(time.Second))
}

func formatBBETimestamp(t time.Time) string {
	if t.IsZero() {
		return goZeroTimestamp
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func (p *SyntheticHTTPProber) registerMetrics(registry *prometheus.Registry, sample Sample) {
	ssl := 0.0
	if sample.SSL {
		ssl = 1
	}

	gauges := []struct {
		name   string
		help   string
		value  float64
		labels prometheus.Labels
	}{
		{"probe_dns_lookup_time_seconds", "Returns the time taken for probe dns lookup in seconds", sample.DNSLookupSeconds, nil},
		{"probe_failed_due_to_regex", "Indicates if probe failed due to regex", sample.FailedDueToRegex, nil},
		{"probe_http_content_length", "Length of http content response", sample.ContentLength, nil},
		{"probe_http_redirects", "The number of redirects", sample.Redirects, nil},
		{"probe_http_ssl", "Indicates if SSL was used for the final redirect", ssl, nil},
		{"probe_http_status_code", "Response HTTP status code", float64(sample.StatusCode), nil},
		{"probe_http_uncompressed_body_length", "Length of uncompressed response body", sample.UncompressedLength, nil},
		{"probe_http_version", "Returns the version of HTTP of the probe response", sample.HTTPVersion, nil},
		{"probe_ip_addr_hash", "Specifies the hash of IP address. It's useful to detect if the IP address changes.", sample.IPAddrHash, nil},
		{"probe_ip_protocol", "Specifies whether probe ip protocol is IP4 or IP6", sample.IPProtocol, nil},
	}

	for _, g := range gauges {
		gauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        g.name,
			Help:        g.help,
			ConstLabels: g.labels,
		})
		registry.MustRegister(gauge)
		gauge.Set(g.value)
	}

	phases := map[string]float64{
		"connect":    sample.ConnectSeconds,
		"processing": sample.ProcessingSeconds,
		"resolve":    sample.ResolveSeconds,
		"tls":        sample.TLSSeconds,
		"transfer":   sample.TransferSeconds,
	}
	for phase, value := range phases {
		gauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "probe_http_duration_seconds",
			Help:        "Duration of http request by phase, summed over all redirects",
			ConstLabels: prometheus.Labels{"phase": phase},
		})
		registry.MustRegister(gauge)
		gauge.Set(value)
	}

	// SSL-specific metrics, mirroring SyntheticTCPProber.registerMetrics'
	// gate exactly, plus probe_tls_cipher_info: the real bbe HTTP prober
	// (prober/http.go's ProbeHTTP, isSSL branch, registry.MustRegister at
	// line 717) registers all five of these only when the connection was
	// SSL -- confirmed against internal/scraper/testdata/http_ssl.txt, which
	// declares probe_ssl_earliest_cert_expiry,
	// probe_ssl_last_chain_expiry_timestamp_seconds, probe_ssl_last_chain_info,
	// probe_tls_version_info, and probe_tls_cipher_info, none of which appear
	// in the plain http.txt fixture.
	if sample.SSL {
		sslGauges := []struct {
			name  string
			help  string
			value float64
		}{
			{"probe_ssl_earliest_cert_expiry", "Returns last SSL chain expiry in unixtime", sample.SSLEarliestCertExpiry},
			{"probe_ssl_last_chain_expiry_timestamp_seconds", "Returns last SSL chain expiry in timestamp", sample.SSLLastChainExpiryTimestampSeconds},
		}

		for _, g := range sslGauges {
			gauge := prometheus.NewGauge(prometheus.GaugeOpts{
				Name: g.name,
				Help: g.help,
			})
			registry.MustRegister(gauge)
			gauge.Set(g.value)
		}

		// probe_ssl_last_chain_info with labels
		chainInfoVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "probe_ssl_last_chain_info",
			Help: "Contains SSL leaf certificate information",
		}, []string{"fingerprint_sha256", "issuer", "serialnumber", "subject", "subjectalternative"})
		registry.MustRegister(chainInfoVec)
		chainInfoVec.WithLabelValues(
			sample.SSLLastChainFingerprint,
			sample.SSLLastChainIssuer,
			sample.SSLLastChainSerialNumber,
			sample.SSLLastChainSubject,
			sample.SSLLastChainSubjectAlternative,
		).Set(1)

		// probe_tls_version_info with labels
		tlsInfoVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "probe_tls_version_info",
			Help: "Returns the TLS version used or NaN when unknown",
		}, []string{"version"})
		registry.MustRegister(tlsInfoVec)
		tlsInfoVec.WithLabelValues(sample.TLSVersion).Set(1)

		// probe_tls_cipher_info with labels -- HTTP-only (TCP/gRPC's
		// registerMetrics do not emit this gauge; confirmed against
		// tcp_ssl.txt/grpc_ssl.txt, neither of which declares it).
		tlsCipherVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "probe_tls_cipher_info",
			Help: "Returns the TLS cipher negotiated during handshake",
		}, []string{"cipher"})
		registry.MustRegister(tlsCipherVec)
		tlsCipherVec.WithLabelValues(sample.TLSCipher).Set(1)
	}
}
