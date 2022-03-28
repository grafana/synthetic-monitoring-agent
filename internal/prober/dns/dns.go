package dns

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	bbeprober "github.com/prometheus/blackbox_exporter/prober"
	"github.com/prometheus/client_golang/prometheus"
)

var errUnsupportedCheck = errors.New("unsupported check")

type Prober struct {
	target string
	config config.Module
}

func NewProber(check sm.Check) (Prober, error) {
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

func (p Prober) Name() string {
	return "dns"
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	// The target of the BBE DNS check is the _DNS server_, while
	// the target of the SM DNS check is the _query_, so we need
	// pass the server as the target parameter, and ignore the
	// _target_ paramater that is passed to this function.
	return bbeprober.ProbeDNS(ctx, p.target, p.config, registry, logger)
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
