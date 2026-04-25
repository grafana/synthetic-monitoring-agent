package scraper

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/prometheus/prompb"
)

// TestCatalogueFromReaderDNSGoldenIncludesSerial proves the DNS golden file is
// exercising the SOA branch that emits probe_dns_serial.
func TestCatalogueFromReaderDNSGoldenIncludesSerial(t *testing.T) {
	fh, err := os.Open(filepath.Join("testdata", "dns.txt"))
	if err != nil {
		t.Fatalf("open dns golden file: %v", err)
	}
	defer fh.Close()

	catalogue, err := CatalogueFromReader(fh)
	if err != nil {
		t.Fatalf("catalogue from reader: %v", err)
	}

	labels, ok := catalogue["probe_dns_serial"]
	if !ok {
		t.Fatalf("probe_dns_serial missing from DNS catalogue")
	}
	if len(labels) != 0 {
		t.Fatalf("probe_dns_serial should not carry raw MetricFamily labels in scraper golden output, got %v", labels)
	}
}

// TestCatalogueFromTimeseries verifies that catalogue extraction merges label
// names across repeated series for the same metric family.
func TestCatalogueFromTimeseries(t *testing.T) {
	catalogue := CatalogueFromTimeseries(TimeSeries{
		{Labels: []prompb.Label{{Name: "__name__", Value: "probe_success"}, {Name: "config_version", Value: "1"}, {Name: "job", Value: "j"}}},
		{Labels: []prompb.Label{{Name: "__name__", Value: "probe_success"}, {Name: "config_version", Value: "2"}, {Name: "probe", Value: "p"}}},
	})

	expected := []string{"config_version", "job", "probe"}
	got := catalogue["probe_success"]
	if len(got) != len(expected) {
		t.Fatalf("unexpected labels: %v", got)
	}
	for i, label := range expected {
		if got[i] != label {
			t.Fatalf("unexpected labels: %v", got)
		}
	}
}

// TestCompareMetricCatalogue covers the mismatch reporting we use for both the
// fixture and runtime catalogue contract tests.
func TestCompareMetricCatalogue(t *testing.T) {
	expected := MetricLabelCatalogue{
		"probe_success":    {"config_version"},
		"probe_dns_serial": {},
	}
	observed := MetricLabelCatalogue{
		"probe_success":    {"config_version", "unexpected_label"},
		"probe_other":      {},
		"probe_dns_serial": {},
	}

	result := CompareMetricCatalogue(expected, observed)
	if result.Success() {
		t.Fatalf("expected mismatch")
	}
	if len(result.MissingMetrics) != 0 {
		t.Fatalf("expected no missing metrics, got %v", result.MissingMetrics)
	}
	if len(result.UnexpectedMetrics) != 1 || result.UnexpectedMetrics[0] != "probe_other" {
		t.Fatalf("unexpected metrics mismatch: %v", result.UnexpectedMetrics)
	}

	mismatch, ok := result.LabelMismatches["probe_success"]
	if !ok {
		t.Fatalf("expected probe_success label mismatch")
	}
	if len(mismatch.MissingLabels) != 0 {
		t.Fatalf("expected no missing labels, got %v", mismatch.MissingLabels)
	}
	if len(mismatch.UnexpectedLabels) != 1 || mismatch.UnexpectedLabels[0] != "unexpected_label" {
		t.Fatalf("unexpected labels mismatch: %v", mismatch.UnexpectedLabels)
	}
}
