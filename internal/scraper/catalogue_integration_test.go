package scraper

import (
	"os"
	"testing"
)

const runtimeCatalogueIntegrationEnv = "SM_RUNTIME_CATALOGUE_INTEGRATION"

// TestValidateMetricCatalogueIntegration validates the runtime-only catalogue
// for k6-backed checks when they execute with a real sm-k6 binary and
// browser-capable environment. It is intentionally distinct from the fixture
// catalogue test, which protects the deterministic local fixture/replay path.
func TestValidateMetricCatalogueIntegration(t *testing.T) {
	if os.Getenv(runtimeCatalogueIntegrationEnv) == "" {
		t.Skipf("set %s=1 to run runtime catalogue integration tests", runtimeCatalogueIntegrationEnv)
	}

	k6Path := os.Getenv("K6_PATH")
	if k6Path == "" {
		t.Fatalf("K6_PATH must be set when %s is enabled", runtimeCatalogueIntegrationEnv)
	}
	if _, err := os.Stat(k6Path); err != nil {
		t.Fatalf("K6_PATH %q is not usable: %v", k6Path, err)
	}

	expectedCatalogues := loadMetricLabelCatalogues(t, "expected_runtime_metric_catalogue.json")

	for class, spec := range runtimeCatalogueSpecs() {
		t.Run(class, func(t *testing.T) {
			t.Setenv("K6_PATH", k6Path)
			expected, ok := expectedCatalogues[class]
			if !ok {
				t.Fatalf("missing expected catalogue for %s", class)
			}

			observed := collectFixtureCatalogue(t, class, spec)
			result := CompareMetricCatalogue(expected, observed)
			if !result.Success() {
				t.Fatalf("catalogue mismatch for %s: %s", class, result.Summary())
			}
		})
	}
}
