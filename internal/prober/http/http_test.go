package http

import (
	"context"
	"io"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	httpConfig "github.com/prometheus/common/config"
	"github.com/rs/zerolog"
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

func TestSettingsToModule(t *testing.T) {
	testcases := map[string]struct {
		input    sm.HttpSettings
		expected config.Module
	}{
		"default": {
			input: sm.HttpSettings{},
			expected: config.Module{
				Prober:  "http",
				Timeout: 0,
				HTTP: config.HTTPProbe{
					ValidStatusCodes:   []int{},
					ValidHTTPVersions:  []string{},
					IPProtocol:         "ip6",
					IPProtocolFallback: true,
					Method:             "GET",
					Headers: map[string]string{
						"user-agent": version.UserAgent(),
					},
					FailIfBodyMatchesRegexp:      []config.Regexp{},
					FailIfBodyNotMatchesRegexp:   []config.Regexp{},
					FailIfHeaderMatchesRegexp:    []config.HeaderMatch{},
					FailIfHeaderNotMatchesRegexp: []config.HeaderMatch{},
					HTTPClientConfig: httpConfig.HTTPClientConfig{
						FollowRedirects: true,
					},
				},
			},
		},
		"partial-settings": {
			input: sm.HttpSettings{
				ValidStatusCodes: []int32{200, 201},
				Method:           5,
				Body:             "This is a body",
			},
			expected: config.Module{
				Prober:  "http",
				Timeout: 0,
				HTTP: config.HTTPProbe{
					ValidStatusCodes:   []int{200, 201},
					ValidHTTPVersions:  []string{},
					IPProtocol:         "ip6",
					IPProtocolFallback: true,
					Method:             "POST",
					Headers: map[string]string{
						"user-agent": version.UserAgent(),
					},
					Body:                         "This is a body",
					FailIfBodyMatchesRegexp:      []config.Regexp{},
					FailIfBodyNotMatchesRegexp:   []config.Regexp{},
					FailIfHeaderMatchesRegexp:    []config.HeaderMatch{},
					FailIfHeaderNotMatchesRegexp: []config.HeaderMatch{},
					HTTPClientConfig: httpConfig.HTTPClientConfig{
						FollowRedirects: true,
					},
				},
			},
		},
	}

	for name, testcase := range testcases {
		ctx := context.TODO()
		logger := zerolog.New(io.Discard)
		t.Run(name, func(t *testing.T) {
			actual, err := settingsToModule(ctx, &testcase.input, logger)
			require.NoError(t, err)
			require.Equal(t, &testcase.expected, &actual)
		})
	}
}
