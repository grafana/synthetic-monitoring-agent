package scraper

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
)

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
