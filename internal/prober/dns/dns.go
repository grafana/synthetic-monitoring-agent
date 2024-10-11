package dns

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/dns/internal/bbe/config"
	bbeprober "github.com/grafana/synthetic-monitoring-agent/internal/prober/dns/internal/bbe/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
)

var errUnsupportedCheck = errors.New("unsupported check")

type Prober struct {
	target       string
	config       config.Module
	experimental bool
}

func NewProber(check model.Check) (Prober, error) {
	if check.Settings.Dns == nil {
		return Prober{}, errUnsupportedCheck
	}

	cfg := settingsToModule(check.Settings.Dns, check.Target)
	cfg.Timeout = time.Duration(check.Timeout) * time.Millisecond

	return Prober{
		target: check.Settings.Dns.Server,
		config: cfg,
	}, nil
}

func NewExperimentalProber(check model.Check) (Prober, error) {
	p, err := NewProber(check)
	if err != nil {
		return p, err
	}

	p.experimental = true

	return p, nil
}

func (p Prober) Name() string {
	return "dns"
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) (bool, float64) {
	cfg := p.config

	if p.experimental {
		const (
			cutoff  = 15 * time.Second
			retries = 3
		)

		if deadline, found := ctx.Deadline(); found {
			budget := time.Until(deadline)
			if budget >= cutoff {
				cfg.DNS.Retries = retries
				// Split 99% of the budget between three retries. For a
				// budget of 15s, this allows for 150 ms per retry for
				// other operations.
				cfg.DNS.RetryTimeout = budget * 99 / (retries * 100)
			}
		}

		_ = logger.Log("msg", "probing DNS", "target", target, "retries", cfg.DNS.Retries, "retry_timeout", cfg.DNS.RetryTimeout)
	}

	// The target of the BBE DNS check is the _DNS server_, while
	// the target of the SM DNS check is the _query_, so we need
	// pass the server as the target parameter, and ignore the
	// _target_ paramater that is passed to this function.

	return bbeprober.ProbeDNS(ctx, p.target, cfg, registry, logger), 0
}

func settingsToModule(settings *sm.DnsSettings, target string) config.Module {
	var m config.Module

	m.Prober = sm.CheckTypeDns.String()
	m.DNS.IPProtocol, m.DNS.IPProtocolFallback = settings.IpVersion.ToIpProtocol()

	// BBE dns_probe actually tests the DNS server, so we
	// need to pass the query (e.g. www.grafana.com) as part
	// of the configuration and the server as the target
	// parameter.
	m.DNS.QueryName = target
	m.DNS.QueryType = settings.RecordType.String()
	m.DNS.SourceIPAddress = settings.SourceIpAddress
	// In the protobuffer definition the protocol is either
	// "TCP" or "UDP", but blackbox-exporter wants "tcp" or
	// "udp".
	m.DNS.TransportProtocol = strings.ToLower(settings.Protocol.String())

	m.DNS.Recursion = true

	m.DNS.ValidRcodes = settings.ValidRCodes

	if settings.ValidateAnswer != nil {
		m.DNS.ValidateAnswer.FailIfMatchesRegexp = settings.ValidateAnswer.FailIfMatchesRegexp
		m.DNS.ValidateAnswer.FailIfNotMatchesRegexp = settings.ValidateAnswer.FailIfNotMatchesRegexp
	}

	if settings.ValidateAuthority != nil {
		m.DNS.ValidateAuthority.FailIfMatchesRegexp = settings.ValidateAuthority.FailIfMatchesRegexp
		m.DNS.ValidateAuthority.FailIfNotMatchesRegexp = settings.ValidateAuthority.FailIfNotMatchesRegexp
	}

	if settings.ValidateAdditional != nil {
		m.DNS.ValidateAdditional.FailIfMatchesRegexp = settings.ValidateAdditional.FailIfMatchesRegexp
		m.DNS.ValidateAdditional.FailIfNotMatchesRegexp = settings.ValidateAdditional.FailIfNotMatchesRegexp
	}

	return m
}
