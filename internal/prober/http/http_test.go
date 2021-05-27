package http

import (
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	"github.com/stretchr/testify/require"
)

func TestBuildHeaders(t *testing.T) {
	testcases := map[string]struct {
		input    []string
		expected map[string]string
	}{
		"nil": {
			input: nil,
			expected: map[string]string{
				"user-agent": version.UserAgent(),
			},
		},

		"empty": {
			input: []string{},
			expected: map[string]string{
				"user-agent": version.UserAgent(),
			},
		},

		"trivial": {
			input: []string{
				"foo: bar",
			},
			expected: map[string]string{
				"foo":        "bar",
				"user-agent": version.UserAgent(),
			},
		},

		"multiple headers": {
			input: []string{
				"h1: v1",
				"h2: v2",
			},
			expected: map[string]string{
				"h1":         "v1",
				"h2":         "v2",
				"user-agent": version.UserAgent(),
			},
		},

		"compact": {
			input: []string{
				"h1:v1",
				"h2:v2",
			},
			expected: map[string]string{
				"h1":         "v1",
				"h2":         "v2",
				"user-agent": version.UserAgent(),
			},
		},

		"trim leading whitespace": {
			input: []string{
				"h1:   v1",
				"h2:      v2",
			},
			expected: map[string]string{
				"h1":         "v1",
				"h2":         "v2",
				"user-agent": version.UserAgent(),
			},
		},

		"keep trailing whitespace": {
			input: []string{
				"h1: v1   ",
				"h2: v2 ",
			},
			expected: map[string]string{
				"h1":         "v1   ",
				"h2":         "v2 ",
				"user-agent": version.UserAgent(),
			},
		},

		"empty values": {
			input: []string{
				"h1: ",
				"h2:",
			},
			expected: map[string]string{
				"h1":         "",
				"h2":         "",
				"user-agent": version.UserAgent(),
			},
		},

		"custom user agent": {
			input: []string{
				"User-Agent: test agent",
			},
			expected: map[string]string{
				"User-Agent": "test agent",
			},
		},

		"custom user agent, weird header capitalization": {
			input: []string{
				"uSEr-AGenT: test agent",
			},
			expected: map[string]string{
				"uSEr-AGenT": "test agent",
			},
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := buildHttpHeaders(testcase.input)
			require.Equal(t, testcase.expected, actual)
		})
	}
}
