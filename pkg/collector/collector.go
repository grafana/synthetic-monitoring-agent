// Package collector exposes the agent's normal single-execution telemetry
// pipeline to callers that supply a prober. It contains no scheduling,
// persistence, or scenario semantics.
package collector

import (
	"context"
	"io"
	"time"

	logproto "github.com/grafana/loki/pkg/push"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	internalprober "github.com/grafana/synthetic-monitoring-agent/internal/prober"
	internallogger "github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/scraper"
	"github.com/grafana/synthetic-monitoring-agent/internal/telemetry"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
)

type TimeSeries = []prompb.TimeSeries
type Streams = []logproto.Stream

// Logger is the logging surface available to an injected Probe.
type Logger interface {
	Log(keyvals ...any) error
}

// Probe performs one check execution and registers its metrics and logs.
// It matches the agent's prober contract without exposing internal packages.
type Probe interface {
	Name() string
	Probe(ctx context.Context, target string, registry *prometheus.Registry, logger Logger) (bool, float64)
}

// Collector runs an injected Probe through the same metric and log
// transformation pipeline used by the agent scraper, without publishing.
type Collector struct {
	scraper *scraper.Scraper
}

// New builds a Collector for one API-level check and probe.
func New(ctx context.Context, check sm.Check, probe sm.Probe, supplied Probe) (*Collector, error) {
	var modelCheck model.Check
	if err := modelCheck.FromSM(check); err != nil {
		return nil, err
	}

	s, err := scraper.NewWithOpts(ctx, modelCheck, scraper.ScraperOpts{
		Probe:                 probe,
		Publisher:             noopPublisher{},
		Logger:                zerolog.New(io.Discard),
		Metrics:               noopMetrics{},
		ProbeFactory:          probeFactory{probe: supplied},
		LabelsLimiter:         noopLabelsLimiter{},
		Telemeter:             noopTelemeter{},
		CostAttributionLabels: noopTenantCals{},
	})
	if err != nil {
		return nil, err
	}
	return &Collector{scraper: s}, nil
}

// Collect runs one execution at logical event time t. A failed probe returns
// its telemetry alongside a non-nil error; a fatal collection error returns no
// telemetry.
func (c *Collector) Collect(ctx context.Context, t time.Time) (TimeSeries, Streams, error) {
	ts, streams, _, _, err := c.scraper.CollectData(ctx, t.UTC())
	return ts, streams, err
}

type probeAdapter struct {
	probe Probe
}

func (p probeAdapter) Name() string { return p.probe.Name() }

func (p probeAdapter) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger internallogger.Logger, _ string) (bool, float64) {
	return p.probe.Probe(ctx, target, registry, logger)
}

type probeFactory struct {
	probe Probe
}

func (f probeFactory) New(_ context.Context, _ zerolog.Logger, check model.Check) (internalprober.Prober, string, error) {
	return probeAdapter{probe: f.probe}, check.Target, nil
}

type noopPublisher struct{}

func (noopPublisher) Publish(pusher.Payload) {}

type noopMetrics struct{}

func (noopMetrics) AddScrape()         {}
func (noopMetrics) AddCheckError()     {}
func (noopMetrics) AddCollectorError() {}

type noopLabelsLimiter struct{}

func (noopLabelsLimiter) MetricLabels(context.Context, model.GlobalID) (int, error) {
	return 128, nil
}

func (noopLabelsLimiter) LogLabels(context.Context, model.GlobalID) (int, error) {
	return 128, nil
}

type noopTelemeter struct{}

func (noopTelemeter) AddExecution(telemetry.Execution) {}

type noopTenantCals struct{}

func (noopTenantCals) CostAttributionLabels(context.Context, model.GlobalID) ([]string, error) {
	return nil, nil
}
