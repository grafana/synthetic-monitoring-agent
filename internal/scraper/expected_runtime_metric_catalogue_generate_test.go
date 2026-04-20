package scraper

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

const generateRuntimeCatalogueEnv = "SM_GENERATE_RUNTIME_CATALOGUE"

// TestPrintExpectedRuntimeMetricCatalogue is an opt-in generator for refreshing
// the runtime catalogue from real sm-k6/browser execution output.
func TestPrintExpectedRuntimeMetricCatalogue(t *testing.T) {
	if os.Getenv(generateRuntimeCatalogueEnv) == "" {
		t.Skipf("set %s=1 to print the runtime metric catalogue", generateRuntimeCatalogueEnv)
	}

	classes := []string{
		"browser",
		"browser_basic",
		"multihttp",
		"multihttp_basic",
		"scripted",
		"scripted_basic",
	}

	catalogues := make(map[string]MetricLabelCatalogue, len(classes))
	for _, class := range classes {
		spec, ok := catalogueFixtureSpecs()[class]
		if !ok {
			t.Fatalf("missing fixture spec for %s", class)
		}
		catalogues[class] = collectFixtureCatalogueSilently(t, class, spec)
	}

	fmt.Println("CATALOGUE_BEGIN")
	fmt.Print(renderExpectedRuntimeMetricCatalogue(catalogues))
	fmt.Println("CATALOGUE_END")
}

func renderExpectedRuntimeMetricCatalogue(catalogues map[string]MetricLabelCatalogue) string {
	var b strings.Builder

	b.WriteString("package scraper\n\n")
	b.WriteString("// expectedRuntimeMetricCatalogueByAccountingClass describes the final published\n")
	b.WriteString("// timeseries label contract for k6-backed checks when they are executed with an\n")
	b.WriteString("// sm-k6 binary and browser-capable runtime.\n")
	b.WriteString("var expectedRuntimeMetricCatalogueByAccountingClass = map[string]MetricLabelCatalogue{\n")

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
