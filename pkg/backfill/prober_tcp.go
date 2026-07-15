package backfill

import (
	"context"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// SyntheticTCPProber registers probe_tcp_*/probe_ip_*/probe_ssl_* metrics from a
// configured TCPSample, mirroring SyntheticHTTPProber's role for HTTP.
type SyntheticTCPProber struct {
	target string
	sample TCPSample
}

func NewSyntheticTCPProber(target string) *SyntheticTCPProber {
	return &SyntheticTCPProber{target: target}
}

func (p *SyntheticTCPProber) SetSample(sample TCPSample) {
	sample.Normalize()
	p.sample = sample
}

// SetTyped implements SyntheticProber. s is expected to be a TypedTCPSample,
// which is the only TypedSample the registry constructs for CheckTypeTcp; any
// other concrete type is a no-op since it cannot describe a TCP sample.
func (p *SyntheticTCPProber) SetTyped(s TypedSample) {
	if typed, ok := s.(TypedTCPSample); ok {
		p.SetSample(typed.Sample)
	}
}

var _ SyntheticProber = (*SyntheticTCPProber)(nil)

func (p *SyntheticTCPProber) Name() string {
	return "tcp"
}

func (p *SyntheticTCPProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	sample := p.sample
	sample.Normalize()
	if target == "" {
		target = p.target
	}

	p.emitLogs(l, target, sample)
	p.registerMetrics(registry, sample)

	return sample.Success, sample.DurationSeconds
}

// emitLogs models the log vocabulary of the real TCP prober
// (github.com/prometheus/blackbox_exporter/prober/{tcp,utils}.go). The real
// message strings and their levels, verified against the vendored module
// (v0.27.0, which uses log/slog -- levels land in Loki as the exact
// slog.Level.String() spelling, i.e. "INFO"/"ERROR", not the lowercase
// go-kit convention the older go-kit-based DNS/ICMP probers use):
//
//   - utils.go chooseProtocol: logger.Info("Resolving target address", ...)
//     then logger.Info("Resolved target address", ...) on success, or
//     logger.Error("Resolution with IP protocol failed", ...) on DNS failure.
//   - tcp.go dialTCP: logger.Info("Dialing TCP without TLS") or
//     logger.Info("Dialing TCP with TLS") depending on module.TCP.TLS.
//   - tcp.go ProbeTCP: on dialer failure, logger.Error("Error dialing TCP",
//     "err", err); on success, logger.Info("Successfully dialed").
//   - tcp.go ProbeTCP (query/response loop, only entered when
//     module.TCP.QueryResponse is configured): logger.Info("Processing
//     query response entry", "entry_number", i), then on regex mismatch
//     logger.Error("Regexp did not match", "regexp", ..., "line", ...).
//
// Per-execution line count (documented for the harness manifest):
//   - success:          4 INFO lines (resolving, resolved, dialing, successfully dialed)
//   - dial/connect failed: 4 lines (3 INFO + 1 ERROR: resolving, resolved, dialing, "Error dialing TCP")
//   - expect mismatch:  6 lines (5 INFO + 1 ERROR: resolving, resolved, dialing, successfully
//     dialed, processing query response entry, "Regexp did not match")
func (p *SyntheticTCPProber) emitLogs(l logger.Logger, target string, sample TCPSample) {
	times := []time.Time{
		sample.At,
		sample.At.Add(1 * time.Millisecond),
		sample.At.Add(2 * time.Millisecond),
		sample.At.Add(3 * time.Millisecond),
		sample.At.Add(4 * time.Millisecond),
		sample.At.Add(5 * time.Millisecond),
	}
	logAt := func(level string, at time.Time, msg string, extra ...any) {
		attrs := []any{"level", level, "msg", msg, "time", at.Format(time.RFC3339Nano)}
		attrs = append(attrs, extra...)
		_ = l.Log(attrs...)
	}
	logInfo := func(at time.Time, msg string, extra ...any) { logAt("INFO", at, msg, extra...) }
	logError := func(at time.Time, msg string, extra ...any) { logAt("ERROR", at, msg, extra...) }

	ipProtocol := "ip4"
	if sample.IPProtocol == 6 {
		ipProtocol = "ip6"
	}
	dialMsg := "Dialing TCP without TLS"
	if sample.SSL {
		dialMsg = "Dialing TCP with TLS"
	}

	logInfo(times[0], "Resolving target address", "target", target, "ip_protocol", ipProtocol)
	logInfo(times[1], "Resolved target address", "target", target, "ip", "127.0.0.1")

	if !sample.Success && sample.FailedDueToRegex == 0 {
		// Dial/connect failed: chooseProtocol resolved fine, but
		// dialer.DialContext (or tls.DialWithDialer) returned an error.
		logInfo(times[2], dialMsg)
		logError(times[3], "Error dialing TCP", "err", "dial tcp: connect: connection refused")
		return
	}

	logInfo(times[2], dialMsg)
	logInfo(times[3], "Successfully dialed")

	if sample.FailedDueToRegex > 0 {
		// Expect mismatch: query/response entry processed, but the
		// configured regexp never matched anything read from the connection.
		logInfo(times[4], "Processing query response entry", "entry_number", 0)
		logError(times[5], "Regexp did not match", "regexp", "^OK", "line", "")
		return
	}
}

func (p *SyntheticTCPProber) registerMetrics(registry *prometheus.Registry, sample TCPSample) {
	failedDueToRegex := 0.0
	if sample.FailedDueToRegex > 0 {
		failedDueToRegex = 1.0
	}

	gauges := []struct {
		name  string
		help  string
		value float64
	}{
		{"probe_dns_lookup_time_seconds", "Returns the time taken for probe dns lookup in seconds", sample.DNSLookupSeconds},
		{"probe_failed_due_to_regex", "Indicates if probe failed due to regex", failedDueToRegex},
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

	// SSL-specific metrics
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
	}
}
