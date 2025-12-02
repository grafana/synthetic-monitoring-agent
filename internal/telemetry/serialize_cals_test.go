package telemetry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSerializeCALs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "nil slice returns __MISSING__",
			input:    nil,
			expected: "__MISSING__",
		},
		{
			name:     "empty slice returns __MISSING__",
			input:    []string{},
			expected: "__MISSING__",
		},
		{
			name:     "single label",
			input:    []string{"team=a"},
			expected: "team=a",
		},
		{
			name:     "multiple labels already sorted",
			input:    []string{"env=prod", "team=a"},
			expected: "env=prod,team=a",
		},
		{
			name:     "multiple labels unsorted",
			input:    []string{"team=a", "env=prod"},
			expected: "env=prod,team=a",
		},
		{
			name:     "three labels unsorted",
			input:    []string{"region=us", "env=prod", "team=a"},
			expected: "env=prod,region=us,team=a",
		},
		{
			name:     "single label with complex value",
			input:    []string{"key=value-with-dashes_and_underscores"},
			expected: "key=value-with-dashes_and_underscores",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := serializeCALs(tt.input)
			require.Equal(t, tt.expected, result)

			// Test immutability: input slice should not be modified (skip for nil)
			if tt.input != nil {
				original := make([]string, len(tt.input))
				copy(original, tt.input)

				_ = serializeCALs(tt.input)

				require.Equal(t, original, tt.input, "input slice should not be modified")
			}
		})
	}
}
