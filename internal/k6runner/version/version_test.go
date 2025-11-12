package version_test

import (
	"slices"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner/version"
)

func TestOverride(t *testing.T) {
	t.Parallel()

	repo := version.Repository{
		Root:     "./testdata/",
		Override: "./testdata/override/sm-k6-custom",
	}

	versions, err := repo.Entries()
	if err != nil {
		t.Fatalf("retrieving entries: %v", err)
	}

	if len(versions) != 1 {
		t.Fatalf("Expected just the overridden version, got %d", len(versions))
	}

	if v := versions[0].Version; v != "v9.9.9" {
		t.Fatalf("Unexpected version %q", v)
	}
}

func TestVersions(t *testing.T) {
	t.Parallel()

	repo := version.Repository{
		Root: "./testdata/",
	}

	versions, err := repo.Entries()
	if err != nil {
		t.Fatalf("retrieving entries: %v", err)
	}

	expected := []string{
		"v1.2.3",
		"v2.0.0",
	}

	if len(versions) != len(expected) {
		t.Fatalf("Expected to find 2 versions, got %d", len(versions))
	}

	for _, ev := range expected {
		if !slices.ContainsFunc(versions, func(v version.Entry) bool { return ev == v.Version }) {
			t.Fatalf("Expected version %q not found in %v", ev, versions)
		}
	}
}
