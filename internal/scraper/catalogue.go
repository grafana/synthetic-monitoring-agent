package scraper

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

type MetricLabelCatalogue map[string][]string

type MetricLabelMismatch struct {
	MissingLabels    []string
	UnexpectedLabels []string
}

type CatalogueComparison struct {
	MissingMetrics    []string
	UnexpectedMetrics []string
	LabelMismatches   map[string]MetricLabelMismatch
}

func CatalogueFromMetricFamilies(mfs []*dto.MetricFamily) MetricLabelCatalogue {
	catalogue := make(MetricLabelCatalogue, len(mfs))

	for _, mf := range mfs {
		if mf == nil || mf.GetName() == "" {
			continue
		}

		labels := make([]string, 0)
		for _, metric := range mf.GetMetric() {
			for _, label := range metric.GetLabel() {
				labels = append(labels, label.GetName())
			}
		}

		catalogue[mf.GetName()] = uniqueSorted(labels)
	}

	return catalogue
}

func CatalogueFromTimeseries(tss TimeSeries) MetricLabelCatalogue {
	catalogue := make(MetricLabelCatalogue)
	for _, ts := range tss {
		metricName := ""
		labels := make([]string, 0, len(ts.Labels))
		for _, label := range ts.Labels {
			if label.Name == "__name__" {
				metricName = label.Value
				continue
			}
			labels = append(labels, label.Name)
		}
		if metricName == "" {
			continue
		}
		catalogue[metricName] = uniqueSorted(append(catalogue[metricName], labels...))
	}
	return catalogue
}

func CatalogueFromReader(r io.Reader) (MetricLabelCatalogue, error) {
	dec := expfmt.NewDecoder(r, expfmt.NewFormat(expfmt.TypeTextPlain))
	mfs := make([]*dto.MetricFamily, 0)

	for {
		mf := new(dto.MetricFamily)
		err := dec.Decode(mf)
		if err == io.EOF {
			return CatalogueFromMetricFamilies(mfs), nil
		}
		if err != nil {
			return nil, fmt.Errorf("decode Prometheus exposition: %w", err)
		}

		mfs = append(mfs, mf)
	}
}

func CompareMetricCatalogue(expected, observed MetricLabelCatalogue) CatalogueComparison {
	result := CatalogueComparison{
		MissingMetrics:    make([]string, 0),
		UnexpectedMetrics: make([]string, 0),
		LabelMismatches:   make(map[string]MetricLabelMismatch),
	}

	for metric, expectedLabels := range expected {
		observedLabels, ok := observed[metric]
		if !ok {
			result.MissingMetrics = append(result.MissingMetrics, metric)
			continue
		}

		missingLabels := difference(expectedLabels, observedLabels)
		unexpectedLabels := difference(observedLabels, expectedLabels)
		if len(missingLabels) == 0 && len(unexpectedLabels) == 0 {
			continue
		}

		result.LabelMismatches[metric] = MetricLabelMismatch{
			MissingLabels:    missingLabels,
			UnexpectedLabels: unexpectedLabels,
		}
	}

	for metric := range observed {
		if _, ok := expected[metric]; ok {
			continue
		}
		result.UnexpectedMetrics = append(result.UnexpectedMetrics, metric)
	}

	sort.Strings(result.MissingMetrics)
	sort.Strings(result.UnexpectedMetrics)

	return result
}

func (r CatalogueComparison) Success() bool {
	return len(r.MissingMetrics) == 0 && len(r.UnexpectedMetrics) == 0 && len(r.LabelMismatches) == 0
}

func (r CatalogueComparison) Summary() string {
	if r.Success() {
		return ""
	}

	parts := make([]string, 0, 3)
	if len(r.MissingMetrics) > 0 {
		parts = append(parts, fmt.Sprintf("missing metrics: %s", strings.Join(r.MissingMetrics, ", ")))
	}
	if len(r.UnexpectedMetrics) > 0 {
		parts = append(parts, fmt.Sprintf("unexpected metrics: %s", strings.Join(r.UnexpectedMetrics, ", ")))
	}
	if len(r.LabelMismatches) > 0 {
		metrics := make([]string, 0, len(r.LabelMismatches))
		for metric := range r.LabelMismatches {
			metrics = append(metrics, metric)
		}
		sort.Strings(metrics)

		mismatches := make([]string, 0, len(metrics))
		for _, metric := range metrics {
			diff := r.LabelMismatches[metric]
			mismatches = append(mismatches, fmt.Sprintf("%s missing=[%s] unexpected=[%s]", metric, strings.Join(diff.MissingLabels, ", "), strings.Join(diff.UnexpectedLabels, ", ")))
		}
		parts = append(parts, fmt.Sprintf("label mismatches: %s", strings.Join(mismatches, "; ")))
	}

	return strings.Join(parts, " | ")
}

func difference(left, right []string) []string {
	result := make([]string, 0)
	for _, item := range uniqueSorted(left) {
		if slices.Contains(right, item) {
			continue
		}
		result = append(result, item)
	}
	return result
}

func uniqueSorted(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}

	sorted := append([]string(nil), items...)
	sort.Strings(sorted)

	out := sorted[:1]
	for _, item := range sorted[1:] {
		if item == out[len(out)-1] {
			continue
		}
		out = append(out, item)
	}

	return out
}
