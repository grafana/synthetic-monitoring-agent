package telemetry

import (
	"testing"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/require"
)

func TestSerializeCALs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []sm.CostAttributionLabel
		expected string
	}{
		{
			name:     "nil slice returns __MISSING__",
			input:    nil,
			expected: CalNilStringTerminator,
		},
		{
			name:     "empty slice returns __MISSING__",
			input:    []sm.CostAttributionLabel{},
			expected: CalNilStringTerminator,
		},
		{
			name:     "single label",
			input:    []sm.CostAttributionLabel{{Name: "team", Value: "a"}},
			expected: "team=a",
		},
		{
			name:     "multiple labels already sorted",
			input:    []sm.CostAttributionLabel{{Name: "env", Value: "prod"}, {Name: "team", Value: "a"}},
			expected: "env=prod,team=a",
		},
		{
			name:     "multiple labels unsorted",
			input:    []sm.CostAttributionLabel{{Name: "team", Value: "a"}, {Name: "env", Value: "prod"}},
			expected: "env=prod,team=a",
		},
		{
			name:     "three labels unsorted",
			input:    []sm.CostAttributionLabel{{Name: "region", Value: "us"}, {Name: "env", Value: "prod"}, {Name: "team", Value: "a"}},
			expected: "env=prod,region=us,team=a",
		},
		{
			name:     "single label with complex value",
			input:    []sm.CostAttributionLabel{{Name: "key", Value: "value-with-dashes_and_underscores"}},
			expected: "key=value-with-dashes_and_underscores",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := serializeCALs(tt.input)
			require.Equal(t, tt.expected, result)

			// Test immutability: input slice should not be modified (skip for nil)
			if tt.input != nil {
				original := make([]sm.CostAttributionLabel, len(tt.input))
				copy(original, tt.input)

				_ = serializeCALs(tt.input)

				require.Equal(t, original, tt.input, "input slice should not be modified")
			}
		})
	}
}
