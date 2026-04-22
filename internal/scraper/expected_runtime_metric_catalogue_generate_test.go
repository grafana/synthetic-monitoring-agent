package scraper

import (
	"fmt"
	"os"
	"testing"
)

const generateRuntimeCatalogueEnv = "SM_GENERATE_RUNTIME_CATALOGUE"

// TestPrintExpectedRuntimeMetricCatalogueJSON is an opt-in generator for
// refreshing the runtime catalogue JSON from real sm-k6/browser execution
// output.
func TestPrintExpectedRuntimeMetricCatalogueJSON(t *testing.T) {
	if os.Getenv(generateRuntimeCatalogueEnv) == "" {
		t.Skipf("set %s=1 to print the runtime metric catalogue JSON", generateRuntimeCatalogueEnv)
	}
	if k6Path := os.Getenv("K6_PATH"); k6Path == "" {
		t.Fatalf("K6_PATH must be set to print the runtime metric catalogue JSON")
	} else if _, err := os.Stat(k6Path); err != nil {
		t.Fatalf("K6_PATH %q is not usable: %v", k6Path, err)
	}

	catalogues := make(map[string]MetricLabelCatalogue, len(runtimeCatalogueSpecs()))
	for _, class := range sortedCatalogueClasses(runtimeCatalogueSpecs()) {
		spec, ok := runtimeCatalogueSpecs()[class]
		if !ok {
			t.Fatalf("missing runtime fixture spec for %s", class)
		}
		catalogues[class] = collectFixtureCatalogueSilently(t, class, spec)
	}

	fmt.Println("CATALOGUE_BEGIN")
	fmt.Print(renderMetricCatalogueJSON(catalogues))
	fmt.Println("CATALOGUE_END")
}
