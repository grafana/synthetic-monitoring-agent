package k6version

import (
	"testing"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

func TestToK6Versions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		input    []string
		expected []sm.K6Version
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: []sm.K6Version{},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []sm.K6Version{},
		},
		{
			name:  "single version",
			input: []string{"v1.2.3"},
			expected: []sm.K6Version{
				{Version: "v1.2.3"},
			},
		},
		{
			name:  "multiple versions",
			input: []string{"v1.2.3", "v2.0.0", "v0.5.0"},
			expected: []sm.K6Version{
				{Version: "v1.2.3"},
				{Version: "v2.0.0"},
				{Version: "v0.5.0"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := toK6Versions(tc.input)

			if len(got) != len(tc.expected) {
				t.Fatalf("expected %d versions, got %d: %v", len(tc.expected), len(got), got)
			}

			for i, v := range got {
				if v.Version != tc.expected[i].Version {
					t.Fatalf("versions[%d]: expected %q, got %q", i, tc.expected[i].Version, v.Version)
				}
			}
		})
	}
}
