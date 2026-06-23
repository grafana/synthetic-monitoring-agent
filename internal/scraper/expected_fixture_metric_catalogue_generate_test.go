package scraper

import (
	"fmt"
	"os"
	"sort"
	"testing"
)

const generateFixtureCatalogueEnv = "SM_GENERATE_FIXTURE_CATALOGUE"

// TestPrintExpectedFixtureMetricCatalogueJSON is an opt-in generator for
// refreshing the fixture catalogue JSON from the current deterministic scraper
// fixture matrix.
func TestPrintExpectedFixtureMetricCatalogueJSON(t *testing.T) {
	if os.Getenv(generateFixtureCatalogueEnv) == "" {
		t.Skipf("set %s=1 to print the fixture metric catalogue JSON", generateFixtureCatalogueEnv)
	}

	classes := make([]string, 0, len(fixtureCatalogueSpecs()))
	for class := range fixtureCatalogueSpecs() {
		classes = append(classes, class)
	}
	sort.Strings(classes)

	catalogues := make(map[string]MetricLabelCatalogue, len(classes))
	for _, class := range classes {
		catalogues[class] = collectFixtureCatalogueSilently(t, class, fixtureCatalogueSpecs()[class])
	}

	fmt.Println("CATALOGUE_BEGIN")
	fmt.Print(renderMetricCatalogueJSON(catalogues))
	fmt.Println("CATALOGUE_END")
}
