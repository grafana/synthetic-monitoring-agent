package tcp

import (
	"context"
	"errors"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/tls"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	bbeprober "github.com/prometheus/blackbox_exporter/prober"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

var errUnsupportedCheck = errors.New("unsupported check")

type Prober struct {
	config config.Module
}

func NewProber(ctx context.Context, check sm.Check, logger zerolog.Logger) (Prober, error) {
	if check.Settings.Tcp == nil {
		return Prober{}, errUnsupportedCheck
	}

	cfg, err := settingsToModule(ctx, check.Settings.Tcp, logger)
	if err != nil {
		return Prober{}, err
	}

	cfg.Timeout = time.Duration(check.Timeout) * time.Millisecond

	return Prober{
		config: cfg,
	}, nil
}

func (p Prober) Name() string {
	return "tcp"
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	return bbeprober.ProbeTCP(ctx, target, p.config, registry, logger)
}

func settingsToModule(ctx context.Context, settings *sm.TcpSettings, logger zerolog.Logger) (config.Module, error) {
	var m config.Module

	m.Prober = sm.CheckTypeTcp.String()

	m.TCP.IPProtocol, m.TCP.IPProtocolFallback = settings.IpVersion.ToIpProtocol()

	m.TCP.SourceIPAddress = settings.SourceIpAddress

	m.TCP.TLS = settings.Tls

	m.TCP.QueryResponse = make([]config.QueryResponse, len(settings.QueryResponse))

	for _, qr := range settings.QueryResponse {
		re, err := config.NewRegexp(string(qr.Expect))
		if err != nil {
			return m, err
		}

		m.TCP.QueryResponse = append(m.TCP.QueryResponse, config.QueryResponse{
			Expect: re,
			Send:   string(qr.Send),
		})
	}

	if settings.TlsConfig != nil {
		var err error
		m.TCP.TLSConfig, err = tls.SMtoProm(ctx, logger.With().Str("prober", m.Prober).Logger(), settings.TlsConfig)
		if err != nil {
			return m, err
		}
	}

	return m, nil
}
