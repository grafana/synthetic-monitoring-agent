package grpc

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
	if check.Settings.Grpc == nil {
		return Prober{}, errUnsupportedCheck
	}

	cfg, err := settingsToModule(ctx, check.Settings.Grpc, logger)
	if err != nil {
		return Prober{}, err
	}

	cfg.Timeout = time.Duration(check.Timeout) * time.Millisecond

	return Prober{
		config: cfg,
	}, nil
}

func (p Prober) Name() string {
	return "grpc"
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	return bbeprober.ProbeGRPC(ctx, target, p.config, registry, logger)
}

func settingsToModule(ctx context.Context, settings *sm.GrpcSettings, logger zerolog.Logger) (config.Module, error) {
	var m config.Module

	m.Prober = sm.CheckTypeGrpc.String()

	m.GRPC.PreferredIPProtocol, m.GRPC.IPProtocolFallback = settings.IpVersion.ToIpProtocol()

	m.GRPC.Service = settings.Service

	m.GRPC.TLS = settings.Tls

	if settings.TlsConfig != nil {
		var err error
		m.GRPC.TLSConfig, err = tls.SMtoProm(ctx, logger.With().Str("prober", m.Prober).Logger(), settings.TlsConfig)
		if err != nil {
			return m, err
		}
	}

	return m, nil
}
