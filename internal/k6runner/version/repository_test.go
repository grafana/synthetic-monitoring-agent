package version_test

import (
	"errors"
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

	if v := versions[0].Version; v.String() != "9.9.9" {
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
		"1.2.3",
		"2.0.0",
	}

	if len(versions) != len(expected) {
		t.Fatalf("Expected to find 2 versions, got %d", len(versions))
	}

	for _, ev := range expected {
		if !slices.ContainsFunc(versions, func(v version.Entry) bool { return ev == v.Version.String() }) {
			t.Fatalf("Expected version %q not found in %v", ev, versions)
		}
	}
}

func TestBinaryFor(t *testing.T) {
	t.Parallel()

	repo := version.Repository{
		Root: "./testdata/",
	}

	// Testdata folder contains k6 mocks matching v1.2.3 and v2.0.0

	for _, tc := range []struct {
		name        string
		constraint  string
		expected    string
		expectError error
	}{
		{
			name:       "Matches anything to most recent",
			constraint: "*",
			expected:   "testdata/sm-k6-v2",
		},
		{
			name:       "Matches caret v1 to v1",
			constraint: "^v1.0.0",
			expected:   "testdata/sm-k6-v1",
		},
		{
			name:       "Matches caret 1 without v to v1",
			constraint: "^1.0.0",
			expected:   "testdata/sm-k6-v1",
		},
		{
			name:       "Matches caret v2 to v2",
			constraint: "^v2.0.0",
			expected:   "testdata/sm-k6-v2",
		},
		{
			name:       "Matches tilde v1 to minor",
			constraint: "~v1.2.0",
			expected:   "testdata/sm-k6-v1",
		},
		{
			name:       "Matches exact version",
			constraint: "v1.2.3",
			expected:   "testdata/sm-k6-v1",
		},
		{
			name:        "Errors for unsatisfiable version",
			constraint:  "^v9.0.0",
			expectError: version.ErrUnsatisfiable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual, err := repo.BinaryFor(tc.constraint)
			if !errors.Is(err, tc.expectError) {
				t.Fatalf("expected error to be %q, got: %q", tc.expectError, err)
			}

			if actual != tc.expected {
				t.Fatalf("expected binary to be %q, got %q", tc.expected, actual)
			}
		})
	}
}
