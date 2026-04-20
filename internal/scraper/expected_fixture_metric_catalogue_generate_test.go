package scraper

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

const generateFixtureCatalogueEnv = "SM_GENERATE_FIXTURE_CATALOGUE"

// TestPrintExpectedMetricCatalogue is an opt-in generator for refreshing the
// fixture catalogue from the current deterministic scraper fixture matrix.
func TestPrintExpectedMetricCatalogue(t *testing.T) {
	if os.Getenv(generateFixtureCatalogueEnv) == "" {
		t.Skipf("set %s=1 to print the fixture metric catalogue", generateFixtureCatalogueEnv)
	}

	classes := make([]string, 0, len(catalogueFixtureSpecs()))
	for class := range catalogueFixtureSpecs() {
		classes = append(classes, class)
	}
	sort.Strings(classes)

	catalogues := make(map[string]MetricLabelCatalogue, len(classes))
	for _, class := range classes {
		catalogues[class] = collectFixtureCatalogueSilently(t, class, catalogueFixtureSpecs()[class])
	}

	fmt.Println("CATALOGUE_BEGIN")
	fmt.Print(renderExpectedMetricCatalogue(catalogues))
	fmt.Println("CATALOGUE_END")
}

func renderExpectedMetricCatalogue(catalogues map[string]MetricLabelCatalogue) string {
	var b strings.Builder

	b.WriteString("package scraper\n\n")
	b.WriteString("// expectedFixtureMetricCatalogueByAccountingClass describes the final published timeseries\n")
	b.WriteString("// label contract after Scraper.collectData applies shared SM labels.\n")
	b.WriteString("//\n")
	b.WriteString("// Traceroute classes are intentionally omitted for now because the repository's\n")
	b.WriteString("// Linux-backed test environment used here lacks the capabilities required to run\n")
	b.WriteString("// traceroute probes end-to-end.\n")
	b.WriteString("var expectedFixtureMetricCatalogueByAccountingClass = map[string]MetricLabelCatalogue{\n")

	classes := make([]string, 0, len(catalogues))
	for class := range catalogues {
		classes = append(classes, class)
	}
	sort.Strings(classes)

	for _, class := range classes {
		fmt.Fprintf(&b, "\t%q: {\n", class)

		metrics := make([]string, 0, len(catalogues[class]))
		for metric := range catalogues[class] {
			metrics = append(metrics, metric)
		}
		sort.Strings(metrics)

		for _, metric := range metrics {
			labels := catalogues[class][metric]
			fmt.Fprintf(&b, "\t\t%q: %#v,\n", metric, labels)
		}
		b.WriteString("\t},\n")
	}

	b.WriteString("}\n")
	return b.String()
}
