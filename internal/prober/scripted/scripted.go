package scripted

import (
	"context"
	"errors"

	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

const proberName = "scripted"

var errUnsupportedCheck = errors.New("unsupported check")

type Module struct {
	Prober string
	Script k6runner.Script
}

type Prober struct {
	logger    zerolog.Logger
	module    Module
	processor *k6runner.Processor
}

func NewProber(ctx context.Context, check model.Check, logger zerolog.Logger, runner k6runner.Runner, store secrets.SecretProvider) (Prober, error) {
	var p Prober

	if check.Settings.Scripted == nil {
		return p, errUnsupportedCheck
	}

	secretStore, err := store.GetSecretCredentials(ctx, check.GlobalTenantID())
	if err != nil {
		return p, err
	}

	p.module = Module{
		Prober: sm.CheckTypeScripted.String(),
		Script: k6runner.Script{
			Script: check.Settings.Scripted.Script,
			Settings: k6runner.Settings{
				Timeout: check.Timeout,
			},
			CheckInfo: k6runner.CheckInfoFromSM(check),
		},
	}

	if secretStore != nil {
		p.module.Script.SecretStore = k6runner.SecretStore{
			Url:   secretStore.Url,
			Token: secretStore.Token,
		}
	}

	processor, err := k6runner.NewProcessor(p.module.Script, runner)
	if err != nil {
		return p, err
	}

	p.processor = processor
	p.logger = logger

	return p, nil
}

func (p Prober) Name() string {
	return proberName
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) (bool, float64) {
	success, err := p.processor.Run(ctx, registry, logger, p.logger)
	if err != nil {
		p.logger.Error().Err(err).Msg("running probe")
		return false, 0
	}

	// TODO(mem): implement custom duration extraction.
	return success, 0
}
