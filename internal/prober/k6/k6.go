package k6

import (
	"context"
	"errors"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

const proberName = "k6"

var errUnsupportedCheck = errors.New("unsupported check")

type Module struct {
	Prober  string
	Timeout time.Duration
	Script  []byte
}

type Prober struct {
	logger zerolog.Logger
	config Module
	script *k6runner.Script
}

func NewProber(ctx context.Context, check sm.Check, logger zerolog.Logger, runner k6runner.Runner) (Prober, error) {
	var p Prober

	if check.Settings.K6 == nil {
		return p, errUnsupportedCheck
	}

	p.config = settingsToModule(check.Settings.K6)
	timeout := time.Duration(check.Timeout) * time.Millisecond
	p.config.Timeout = timeout

	script, err := k6runner.NewScript(check.Settings.K6.Script, runner)
	if err != nil {
		return p, err
	}

	p.script = script
	p.logger = logger

	return p, nil
}

func (p Prober) Name() string {
	return proberName
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	success, err := p.script.Run(ctx, registry, logger, p.logger)
	if err != nil {
		p.logger.Warn().Err(err).Msg("running probe")
		return false
	}

	return success
}

func settingsToModule(settings *sm.K6Settings) Module {
	var m Module

	m.Prober = sm.CheckTypeK6.String()

	m.Script = settings.Script

	return m
}
