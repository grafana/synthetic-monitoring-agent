package browser

import (
	"context"
	"errors"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

const proberName = "browser"

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

func NewProber(ctx context.Context, check sm.Check, logger zerolog.Logger, runner k6runner.Runner) (Prober, error) {
	var p Prober

	if check.Settings.Browser == nil {
		return p, errUnsupportedCheck
	}

	p.module = Module{
		Prober: sm.CheckTypeBrowser.String(),
		Script: k6runner.Script{
			Script: check.Settings.Browser.Script,
			Settings: k6runner.Settings{
				Timeout: check.Timeout,
			},
			// TODO: Add metadata & features here.
		},
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

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	success, err := p.processor.Run(ctx, registry, logger, p.logger)
	if err != nil {
		p.logger.Error().Err(err).Msg("running probe")
		return false
	}

	return success
}
