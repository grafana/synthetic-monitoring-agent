package scraper

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
)

type fixtureSpec struct {
	setup            func(context.Context, *testing.T) (prober.Prober, model.Check, func())
	basicMetricsOnly bool
}

func catalogueFixtureSpecs() map[string]fixtureSpec {
	return map[string]fixtureSpec{
		"browser":         {setup: setupBrowserProbe, basicMetricsOnly: false},
		"browser_basic":   {setup: setupBrowserProbe, basicMetricsOnly: true},
		"dns":             {setup: setupDNSProbe, basicMetricsOnly: false},
		"dns_basic":       {setup: setupDNSProbe, basicMetricsOnly: true},
		"grpc":            {setup: setupGRPCProbe, basicMetricsOnly: false},
		"grpc_basic":      {setup: setupGRPCProbe, basicMetricsOnly: true},
		"grpc_ssl":        {setup: setupGRPCSSLProbe, basicMetricsOnly: false},
		"grpc_ssl_basic":  {setup: setupGRPCSSLProbe, basicMetricsOnly: true},
		"http":            {setup: setupHTTPProbe, basicMetricsOnly: false},
		"http_basic":      {setup: setupHTTPProbe, basicMetricsOnly: true},
		"http_ssl":        {setup: setupHTTPSSLProbe, basicMetricsOnly: false},
		"http_ssl_basic":  {setup: setupHTTPSSLProbe, basicMetricsOnly: true},
		"multihttp":       {setup: setupMultiHTTPProbe, basicMetricsOnly: false},
		"multihttp_basic": {setup: setupMultiHTTPProbe, basicMetricsOnly: true},
		"ping":            {setup: setupPingProbe, basicMetricsOnly: false},
		"ping_basic":      {setup: setupPingProbe, basicMetricsOnly: true},
		"scripted":        {setup: setupScriptedProbe, basicMetricsOnly: false},
		"scripted_basic":  {setup: setupScriptedProbe, basicMetricsOnly: true},
		"tcp":             {setup: setupTCPProbe, basicMetricsOnly: false},
		"tcp_basic":       {setup: setupTCPProbe, basicMetricsOnly: true},
		"tcp_ssl":         {setup: setupTCPSSLProbe, basicMetricsOnly: false},
		"tcp_ssl_basic":   {setup: setupTCPSSLProbe, basicMetricsOnly: true},
	}
}

func collectFixtureCatalogue(t *testing.T, name string, spec fixtureSpec) MetricLabelCatalogue {
	return collectFixtureCatalogueWithLogger(t, name, spec, testhelper.Logger(t))
}

func collectFixtureCatalogueSilently(t *testing.T, name string, spec fixtureSpec) MetricLabelCatalogue {
	return collectFixtureCatalogueWithLogger(t, name, spec, zerolog.New(io.Discard))
}

func collectFixtureCatalogueWithLogger(t *testing.T, name string, spec fixtureSpec, logger zerolog.Logger) MetricLabelCatalogue {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	probeImpl, check, stop := spec.setup(ctx, t)
	defer stop()

	check.Job = name + " job"
	check.Frequency = 60000
	check.Modified = 42
	check.BasicMetricsOnly = spec.basicMetricsOnly

	s := Scraper{
		checkName:     check.Type().String(),
		target:        check.Target,
		logger:        logger,
		prober:        probeImpl,
		labelsLimiter: testLabelsLimiter{maxMetricLabels: 100, maxLogLabels: 100},
		summaries:     make(map[uint64]prometheus.Summary),
		histograms:    make(map[uint64]prometheus.Histogram),
		check:         check,
		probe:         sm.Probe{Id: 100, TenantId: 200, Name: "test-probe", Latitude: -1, Longitude: -2, Region: "test-region"},
	}

	data, _, err := s.collectData(ctx, time.Unix(3141, 0))
	if err != nil {
		t.Fatalf("collectData(%s): %v", name, err)
	}

	return CatalogueFromTimeseries(data.Metrics())
}
