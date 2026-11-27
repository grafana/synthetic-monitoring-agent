package grpc

import (
	"context"
	"errors"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/interpolation"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
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
	// tlsConfig is set only when it references a secret, which then resolves on every run.
	tlsConfig   *sm.TLSConfig
	secretStore secrets.SecretProvider
	tenantID    model.GlobalID
	logger      zerolog.Logger
}

func NewProber(ctx context.Context, check model.Check, logger zerolog.Logger, secretStore secrets.SecretProvider) (Prober, error) {
	if check.Settings.Grpc == nil {
		return Prober{}, errUnsupportedCheck
	}

	cfg, err := settingsToModule(ctx, check.Settings.Grpc, logger)
	if err != nil {
		return Prober{}, err
	}

	cfg.Timeout = time.Duration(check.Timeout) * time.Millisecond

	p := Prober{config: cfg}

	if check.ReferencesSecrets() {
		p.tlsConfig = check.Settings.Grpc.TlsConfig
		p.secretStore = secretStore
		p.tenantID = check.GlobalTenantID()
		p.logger = logger.With().Str("prober", p.Name()).Logger()
	}

	return p, nil
}

func (p Prober) Name() string {
	return "grpc"
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	cfg := p.config

	if p.tlsConfig != nil {
		resolver := interpolation.NewResolver(p.secretStore, p.tenantID, p.logger)

		var err error

		cfg.GRPC.TLSConfig, err = tls.ResolveSMtoProm(ctx, p.logger, p.tlsConfig, resolver)

		l = logger.Redacting(l, resolver.Redactor())

		if err != nil {
			p.logger.Error().Err(err).Msg("failed to resolve secrets for gRPC probe")
			interpolation.LogSecretError(l, err)

			return false, 0
		}
	}

	return bbeprober.ProbeGRPC(ctx, target, cfg, registry, logger.ToSlog(l)), 0
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
