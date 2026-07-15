package backfill

import (
	"context"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// SyntheticGRPCProber registers probe_grpc_*/probe_ip_*/probe_ssl_* metrics from a
// configured GRPCSample, mirroring SyntheticHTTPProber's role for HTTP.
type SyntheticGRPCProber struct {
	target string
	sample GRPCSample
}

func NewSyntheticGRPCProber(target string) *SyntheticGRPCProber {
	return &SyntheticGRPCProber{target: target}
}

func (p *SyntheticGRPCProber) SetSample(sample GRPCSample) {
	sample.Normalize()
	p.sample = sample
}

// SetTyped implements SyntheticProber. s is expected to be a TypedGRPCSample,
// which is the only TypedSample the registry constructs for CheckTypeGrpc; any
// other concrete type is a no-op since it cannot describe a gRPC sample.
func (p *SyntheticGRPCProber) SetTyped(s TypedSample) {
	if typed, ok := s.(TypedGRPCSample); ok {
		p.SetSample(typed.Sample)
	}
}

var _ SyntheticProber = (*SyntheticGRPCProber)(nil)

func (p *SyntheticGRPCProber) Name() string {
	return "grpc"
}

func (p *SyntheticGRPCProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	sample := p.sample
	sample.Normalize()
	if target == "" {
		target = p.target
	}

	p.emitLogs(l, target, sample)
	p.registerMetrics(registry, sample)

	return sample.Success, sample.DurationSeconds
}

// emitLogs models the log vocabulary of the real gRPC prober
// (github.com/prometheus/blackbox_exporter/prober/{grpc,utils}.go), verified
// against the vendored module (v0.27.0, log/slog-based -- levels land in
// Loki as the exact slog.Level.String() spelling: "INFO"/"DEBUG"/"ERROR"):
//
//   - utils.go chooseProtocol: logger.Info("Resolving target address", ...)
//     then logger.Info("Resolved target address", ...) on success.
//   - grpc.go ProbeGRPC never logs anything about "establishing" the
//     connection or the health-check outcome by name -- that vocabulary was
//     fabricated. What production actually does after the resolve step is:
//     `ok, _, _, err := client.Check(...)`, then
//     `if !ok || err != nil { logger.Error("can't connect grpc server:",
//     "err", err); success = false } else {
//     logger.Debug("connect the grpc server successfully"); success = true }`.
//   - Notably this means a "clean" failed health check (RPC succeeded, but
//     returned e.g. NOT_SERVING, so err == nil) logs the exact same
//     "can't connect grpc server:" message as an actual connection failure
//     (with err=<nil> instead of a real error) -- a genuine production quirk
//     mirrored here rather than "fixed" into a more informative message.
//   - The serving status (SERVING/NOT_SERVING/...) is never logged; it only
//     ever appears as a probe_grpc_healthcheck_response gauge label.
//   - A successful check logs at DEBUG, not INFO -- another real quirk (a
//     no-news-is-good-news success is logged more quietly than the resolve
//     steps that precede it).
//
// Per-execution line count (documented for the harness manifest):
//   - success:              3 lines (2 INFO + 1 DEBUG: resolving, resolved, "connect the grpc server successfully")
//   - connect failed:       3 lines (2 INFO + 1 ERROR: resolving, resolved, "can't connect grpc server:")
//   - health check failed:  3 lines (2 INFO + 1 ERROR: resolving, resolved, "can't connect grpc server:")
//
// (Connect-failed and health-check-failed are indistinguishable in the log
// stream -- both are the same message shape, differing only in the "err"
// value -- exactly mirroring production; GRPCSample.ConnectFailed selects
// which "err" value renders.)
func (p *SyntheticGRPCProber) emitLogs(l logger.Logger, target string, sample GRPCSample) {
	times := []time.Time{
		sample.At,
		sample.At.Add(1 * time.Millisecond),
		sample.At.Add(2 * time.Millisecond),
	}
	logAt := func(level string, at time.Time, msg string, extra ...any) {
		attrs := []any{"level", level, "msg", msg, "time", at.Format(time.RFC3339Nano)}
		attrs = append(attrs, extra...)
		_ = l.Log(attrs...)
	}
	logInfo := func(at time.Time, msg string, extra ...any) { logAt("INFO", at, msg, extra...) }
	logError := func(at time.Time, msg string, extra ...any) { logAt("ERROR", at, msg, extra...) }
	logDebug := func(at time.Time, msg string, extra ...any) { logAt("DEBUG", at, msg, extra...) }

	ipProtocol := "ip4"
	if sample.IPProtocol == 6 {
		ipProtocol = "ip6"
	}

	logInfo(times[0], "Resolving target address", "target", target, "ip_protocol", ipProtocol)
	logInfo(times[1], "Resolved target address", "target", target, "ip", "127.0.0.1")

	if !sample.Success {
		errVal := "<nil>"
		if sample.ConnectFailed {
			errVal = "rpc error: code = Unavailable desc = connection error: desc = \"transport: Error while dialing\""
		}
		logError(times[2], "can't connect grpc server:", "err", errVal)
		return
	}

	logDebug(times[2], "connect the grpc server successfully")
}

func (p *SyntheticGRPCProber) registerMetrics(registry *prometheus.Registry, sample GRPCSample) {
	ssl := 0.0
	if sample.SSL {
		ssl = 1.0
	}

	gauges := []struct {
		name  string
		help  string
		value float64
	}{
		{"probe_dns_lookup_time_seconds", "Returns the time taken for probe dns lookup in seconds", sample.DNSLookupSeconds},
		{"probe_grpc_ssl", "Indicates if SSL was used for the connection", ssl},
		{"probe_grpc_status_code", "Response gRPC status code", float64(sample.StatusCode)},
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

	// probe_grpc_duration_seconds with phases
	phases := map[string]float64{
		"resolve": sample.DNSLookupSeconds,
		"check":   sample.GRPCDuration,
	}
	durationVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_grpc_duration_seconds",
		Help: "Duration of gRPC request by phase",
	}, []string{"phase"})
	registry.MustRegister(durationVec)
	for phase, value := range phases {
		durationVec.WithLabelValues(phase).Set(value)
	}

	// probe_grpc_healthcheck_response with serving_status labels
	healthCheckStatuses := []string{"UNKNOWN", "SERVING", "NOT_SERVING", "SERVICE_UNKNOWN"}
	healthCheckVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_grpc_healthcheck_response",
		Help: "Response HealthCheck response",
	}, []string{"serving_status"})
	registry.MustRegister(healthCheckVec)
	for i, status := range healthCheckStatuses {
		value := 0.0
		if i == sample.HealthCheckResponse {
			value = 1.0
		}
		healthCheckVec.WithLabelValues(status).Set(value)
	}

	// SSL-specific metrics
	if sample.SSL {
		sslGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "probe_ssl_earliest_cert_expiry",
			Help: "Returns last SSL chain expiry in unixtime",
		})
		registry.MustRegister(sslGauge)
		sslGauge.Set(sample.SSLEarliestCertExpiry)

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
