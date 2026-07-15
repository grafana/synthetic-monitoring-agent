package backfill

import (
	"context"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// SyntheticDNSProber registers probe_dns_*/probe_ip_* metrics from a
// configured DNSSample, mirroring SyntheticHTTPProber's role for HTTP.
type SyntheticDNSProber struct {
	target string
	sample DNSSample
}

func NewSyntheticDNSProber(target string) *SyntheticDNSProber {
	return &SyntheticDNSProber{target: target}
}

func (p *SyntheticDNSProber) SetSample(sample DNSSample) {
	sample.Normalize()
	p.sample = sample
}

// SetTyped implements SyntheticProber. s is expected to be a TypedDNSSample,
// which is the only TypedSample the registry constructs for CheckTypeDns; any
// other concrete type is a no-op since it cannot describe a DNS sample.
func (p *SyntheticDNSProber) SetTyped(s TypedSample) {
	if typed, ok := s.(TypedDNSSample); ok {
		p.SetSample(typed.Sample)
	}
}

var _ SyntheticProber = (*SyntheticDNSProber)(nil)

func (p *SyntheticDNSProber) Name() string {
	return "dns"
}

// Query metadata used only for log lines; the synthetic DNS sample has no
// dedicated fields for these since the fixture/scraper contract doesn't need
// them to drive any gauge.
const (
	dnsLogQueryType  = "A"
	dnsLogQueryClass = "IN"
)

func (p *SyntheticDNSProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	sample := p.sample
	sample.Normalize()
	if target == "" {
		target = p.target
	}

	p.emitLogs(l, target, sample)
	p.registerMetrics(registry, sample)

	return sample.Success, sample.DurationSeconds
}

// emitLogs models the log vocabulary of the real DNS prober
// (blackbox_exporter@v0.27.0/prober/dns.go, vendored via
// internal/prober/dns): resolving the server (via the shared chooseProtocol
// helper in utils.go, which -- like the TCP/gRPC probers' resolve step --
// logs both "Resolving target address" and "Resolved target address"),
// making the query, getting a response, and validating the RRsets.
//
// Fix (Phase B carried finding): a successful query that passes rcode and
// Answer-RRs validation does NOT return early in the real prober — it goes
// on to validate the Authority and Additional RRsets too
// (dns.go:305-314: "Validating Authority RRs" / "Validating Additional RRs",
// each followed by a validRRs call). Those two lines were previously
// missing, undercounting real success-path log volume by 2. Only the
// success path gains lines: both the query-failure and rcode/validation-
// failure paths already return before reaching Authority/Additional
// validation in the real code (dns.go:297-304), so their counts are
// unchanged.
//
// Per-execution line count (documented for the harness manifest):
//   - success:            7 INFO lines (resolving, resolved, query, response, answer validation OK, authority validation OK, additional validation OK)
//   - query failure:      4 lines (3 INFO + 1 ERROR: resolving, resolved, query, query error)
//   - rcode/validation failure: 5 lines (4 INFO + 1 ERROR: resolving, resolved, query, response, validation error)
func (p *SyntheticDNSProber) emitLogs(l logger.Logger, target string, sample DNSSample) {
	times := []time.Time{
		sample.At,
		sample.At.Add(1 * time.Millisecond),
		sample.At.Add(2 * time.Millisecond),
		sample.At.Add(3 * time.Millisecond),
		sample.At.Add(4 * time.Millisecond),
		sample.At.Add(5 * time.Millisecond),
		sample.At.Add(6 * time.Millisecond),
	}
	logAt := func(level string, at time.Time, msg string, extra ...any) {
		attrs := []any{"level", level, "msg", msg, "time", at.Format(time.RFC3339Nano)}
		attrs = append(attrs, extra...)
		_ = l.Log(attrs...)
	}
	logInfo := func(at time.Time, msg string, extra ...any) { logAt("INFO", at, msg, extra...) }
	logError := func(at time.Time, msg string, extra ...any) { logAt("error", at, msg, extra...) }

	logInfo(times[0], "Resolving target address", "target", target)
	logInfo(times[1], "Resolved target address", "target", target, "ip", "127.0.0.1")

	if !sample.QuerySucceeded {
		logInfo(times[2], "Making DNS query", "target", target, "query", target, "type", dnsLogQueryType, "class", dnsLogQueryClass)
		logError(times[3], "Error while sending a DNS query", "err", "query failed")
		return
	}

	logInfo(times[2], "Making DNS query", "target", target, "query", target, "type", dnsLogQueryType, "class", dnsLogQueryClass)
	logInfo(times[3], "Got response", "rcode", sample.Rcode, "answer_rrs", sample.AnswerRRS)

	if !sample.Success {
		logError(times[4], "Answer RRs validation failed", "rcode", sample.Rcode)
		return
	}

	logInfo(times[4], "Validating Answer RRs", "rcode", sample.Rcode, "answer_rrs", sample.AnswerRRS)
	logInfo(times[5], "Validating Authority RRs", "rcode", sample.Rcode, "authority_rrs", sample.AuthorityRRS)
	logInfo(times[6], "Validating Additional RRs", "rcode", sample.Rcode, "additional_rrs", sample.AdditionalRRS)
}

func (p *SyntheticDNSProber) registerMetrics(registry *prometheus.Registry, sample DNSSample) {
	querySucceeded := 0.0
	if sample.QuerySucceeded {
		querySucceeded = 1
	}

	gauges := []struct {
		name  string
		help  string
		value float64
	}{
		{"probe_dns_answer_rrs", "Returns number of entries in the answer resource record list", float64(sample.AnswerRRS)},
		{"probe_dns_authority_rrs", "Returns number of entries in the authority resource record list", float64(sample.AuthorityRRS)},
		{"probe_dns_additional_rrs", "Returns number of entries in the additional resource record list", float64(sample.AdditionalRRS)},
		{"probe_dns_query_succeeded", "Displays whether or not the query was executed successfully", querySucceeded},
		{"probe_dns_serial", "Returns the serial number of the zone", float64(sample.Serial)},
		{"probe_dns_lookup_time_seconds", "Returns the time taken for probe dns lookup in seconds", sample.LookupSeconds},
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

	phases := map[string]float64{
		"resolve": sample.LookupSeconds,
		"connect": sample.ConnectSeconds,
		"request": sample.RequestSeconds,
	}
	durationVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_dns_duration_seconds",
		Help: "Duration of DNS request by phase",
	}, []string{"phase"})
	registry.MustRegister(durationVec)
	for phase, value := range phases {
		durationVec.WithLabelValues(phase).Set(value)
	}
}
