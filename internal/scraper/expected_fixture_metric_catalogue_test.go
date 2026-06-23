package scraper

import "testing"

// TestExpectedMetricCatalogueMatchesFixtureOutputs validates the deterministic
// fixture catalogue used by the regular scraper package tests. This contract is
// intentionally separate from the runtime catalogue integration test, which
// exercises the richer k6/browser execution path with a real sm-k6 binary.
func TestExpectedMetricCatalogueMatchesFixtureOutputs(t *testing.T) {
	expectedCatalogues := loadMetricLabelCatalogues(t, "expected_fixture_metric_catalogue.json")
	seen := make(map[string]struct{}, len(expectedCatalogues))

	for class, spec := range fixtureCatalogueSpecs() {
		seen[class] = struct{}{}
		expected, ok := expectedCatalogues[class]
		if !ok {
			t.Fatalf("missing expected catalogue for accounting class %s", class)
		}

		observed := collectFixtureCatalogue(t, class, spec)
		result := CompareMetricCatalogue(expected, observed)
		if !result.Success() {
			t.Fatalf("catalogue mismatch for %s: %s", class, result.Summary())
		}
	}

	for class := range expectedCatalogues {
		if _, ok := seen[class]; ok {
			continue
		}
		t.Fatalf("expected catalogue contains class without fixture coverage: %s", class)
	}
}
