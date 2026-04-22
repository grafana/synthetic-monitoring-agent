package scraper

import (
	"sort"
	"strconv"
	"strings"
)

func sortedCatalogueClasses(catalogues map[string]fixtureSpec) []string {
	classes := make([]string, 0, len(catalogues))
	for class := range catalogues {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	return classes
}

func renderMetricCatalogueJSON(catalogues map[string]MetricLabelCatalogue) string {
	var b strings.Builder

	b.WriteString("{\n")
	for classIndex, class := range sortedMetricCatalogueClasses(catalogues) {
		if classIndex > 0 {
			b.WriteString(",\n")
		}
		b.WriteString("\t")
		b.WriteString(strconv.Quote(class))
		b.WriteString(": {\n")

		metrics := sortedMetricCatalogueMetrics(catalogues[class])
		for metricIndex, metric := range metrics {
			if metricIndex > 0 {
				b.WriteString(",\n")
			}
			b.WriteString("\t\t")
			b.WriteString(strconv.Quote(metric))
			b.WriteString(": ")
			b.WriteString(renderStringArrayJSON(catalogues[class][metric]))
		}

		b.WriteString("\n\t}")
	}
	b.WriteString("\n}\n")

	return b.String()
}

func sortedMetricCatalogueClasses(catalogues map[string]MetricLabelCatalogue) []string {
	classes := make([]string, 0, len(catalogues))
	for class := range catalogues {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	return classes
}

func sortedMetricCatalogueMetrics(catalogue MetricLabelCatalogue) []string {
	metrics := make([]string, 0, len(catalogue))
	for metric := range catalogue {
		metrics = append(metrics, metric)
	}
	sort.Strings(metrics)
	return metrics
}

func renderStringArrayJSON(values []string) string {
	var b strings.Builder
	b.WriteString("[")
	for i, value := range values {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(value))
	}
	b.WriteString("]")
	return b.String()
}
