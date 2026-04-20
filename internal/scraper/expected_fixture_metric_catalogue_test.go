package scraper

import "testing"

// TestExpectedMetricCatalogueMatchesFixtureOutputs validates the deterministic
// fixture catalogue used by the regular scraper package tests. This contract is
// intentionally separate from the runtime catalogue integration test, which
// exercises the richer k6/browser execution path with a real sm-k6 binary.
func TestExpectedMetricCatalogueMatchesFixtureOutputs(t *testing.T) {
	seen := make(map[string]struct{}, len(expectedFixtureMetricCatalogueByAccountingClass))

	for class, spec := range catalogueFixtureSpecs() {
		seen[class] = struct{}{}
		expected, ok := expectedFixtureMetricCatalogueByAccountingClass[class]
		if !ok {
			t.Fatalf("missing expected catalogue for accounting class %s", class)
		}

		observed := collectFixtureCatalogue(t, class, spec)
		result := CompareMetricCatalogue(expected, observed)
		if !result.Success() {
			t.Fatalf("catalogue mismatch for %s: %s", class, result.Summary())
		}
	}

	for class := range expectedFixtureMetricCatalogueByAccountingClass {
		if _, ok := seen[class]; ok {
			continue
		}
		t.Fatalf("expected catalogue contains class without fixture coverage: %s", class)
	}
}
