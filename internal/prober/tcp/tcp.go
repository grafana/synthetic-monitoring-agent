package tcp

import (
	"context"
	"errors"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
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

func NewProber(ctx context.Context, check model.Check, logger zerolog.Logger) (Prober, error) {
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

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	slogger := logger.ToSlog(l)
	return bbeprober.ProbeTCP(ctx, target, p.config, registry, slogger), 0
}

func settingsToModule(ctx context.Context, settings *sm.TcpSettings, logger zerolog.Logger) (config.Module, error) {
	var m config.Module

	m.Prober = sm.CheckTypeTcp.String()

	m.TCP.IPProtocol, m.TCP.IPProtocolFallback = settings.IpVersion.ToIpProtocol()

	m.TCP.SourceIPAddress = settings.SourceIpAddress

	m.TCP.TLS = settings.Tls

	m.TCP.QueryResponse = make([]config.QueryResponse, 0, len(settings.QueryResponse))

	for _, qr := range settings.QueryResponse {
		entry := config.QueryResponse{
			Send:     string(qr.Send),
			StartTLS: qr.StartTLS,
		}

		// An empty expect has to stay a nil regexp: BBE reads a line for
		// every step that carries one, and an empty pattern matches anything.
		if len(qr.Expect) > 0 {
			re, err := config.NewRegexp(string(qr.Expect))
			if err != nil {
				return m, err
			}

			entry.Expect = re
		}

		m.TCP.QueryResponse = append(m.TCP.QueryResponse, entry)
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
