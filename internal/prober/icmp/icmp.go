package icmp

import (
	"context"
	"errors"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	bbeprober "github.com/prometheus/blackbox_exporter/prober"
	"github.com/prometheus/client_golang/prometheus"
)

var errUnsupportedCheck = errors.New("unsupported check")

type Prober struct {
	config config.Module
}

func NewProber(check sm.Check) (Prober, error) {
	if check.Settings.Ping == nil {
		return Prober{}, errUnsupportedCheck
	}

	cfg := settingsToModule(check.Settings.Ping)
	cfg.Timeout = time.Duration(check.Timeout) * time.Millisecond

	return Prober{
		config: cfg,
	}, nil
}

func (p Prober) Name() string {
	return "ping"
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	return bbeprober.ProbeICMP(ctx, target, p.config, registry, logger)
}

func settingsToModule(settings *sm.PingSettings) config.Module {
	var m config.Module

	m.Prober = sm.CheckTypePing.String()

	m.ICMP.IPProtocol, m.ICMP.IPProtocolFallback = settings.IpVersion.ToIpProtocol()

	m.ICMP.SourceIPAddress = settings.SourceIpAddress

	m.ICMP.PayloadSize = int(settings.PayloadSize)

	m.ICMP.DontFragment = settings.DontFragment

	return m
}
