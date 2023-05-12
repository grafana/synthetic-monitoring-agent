package multihttp

import (
	"context"
	"errors"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

const proberName = "multi_http"

var errUnsupportedCheck = errors.New("unsupported check")

type Module struct {
	Prober  string
	Timeout time.Duration
}

type Prober struct {
	logger zerolog.Logger
	config Module
	runner *runner
}

func NewProber(ctx context.Context, check sm.Check, logger zerolog.Logger) (Prober, error) {
	var p Prober

	if check.Settings.Multihttp == nil {
		return p, errUnsupportedCheck
	}

	if err := check.Settings.Multihttp.Validate(); err != nil {
		return p, err
	}

	p.config = settingsToModule(check.Settings.Multihttp)
	p.config.Timeout = time.Duration(check.Timeout) * time.Millisecond

	r, err := newRunner(check.Settings.Multihttp)
	if err != nil {
		return p, err
	}

	p.runner = r
	p.logger = logger

	return p, nil
}

func (p Prober) Name() string {
	return proberName
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	err := p.runner.Run(ctx, registry, logger, p.logger)
	if err != nil {
		p.logger.Warn().Err(err).Msg("running probe")
		return false
	}

	return true
}

func settingsToModule(settings *sm.MultiHttpSettings) Module {
	var m Module

	m.Prober = sm.CheckTypeMultiHttp.String()

	return m
}
