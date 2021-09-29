package http

import (
	"context"
	"io"
	"net/url"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	httpConfig "github.com/prometheus/common/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	name := Prober.Name(Prober{})
	require.Equal(t, name, "http")
}

func TestNewProber(t *testing.T) {
	testcases := map[string]struct {
		input       sm.Check
		expected    Prober
		ExpectError bool
	}{
		"default": {
			input: sm.Check{
				Target: "www.grafana.com",
				Settings: sm.CheckSettings{
					Http: &sm.HttpSettings{},
				},
			},
			expected: Prober{
				config: config.Module{
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
			ExpectError: false,
		},
		"no-settings": {
			input: sm.Check{
				Target: "www.grafana.com",
				Settings: sm.CheckSettings{
					Http: nil,
				},
			},
			expected:    Prober{},
			ExpectError: true,
		},
	}

	for name, testcase := range testcases {
		ctx := context.Background()
		logger := zerolog.New(io.Discard)
		t.Run(name, func(t *testing.T) {
			actual, err := NewProber(ctx, testcase.input, logger)
			require.Equal(t, &testcase.expected, &actual)
			if testcase.ExpectError {
				require.Error(t, err, "unsupported check")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

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
		ctx := context.Background()
		logger := zerolog.New(io.Discard)
		t.Run(name, func(t *testing.T) {
			actual, err := settingsToModule(ctx, &testcase.input, logger)
			require.NoError(t, err)
			require.Equal(t, &testcase.expected, &actual)
		})
	}
}

func TestAddCacheBustParam(t *testing.T) {
	target := "www.grafana.com"
	paramName := "test"
	salt := "12345"

	newUrl := addCacheBustParam(target, paramName, salt)
	require.NotEqual(t, target, newUrl)

	// Parse query params and make sure "test" is present
	newUrlQuery, err := url.Parse(newUrl)
	require.Nil(t, err)
	queryString, err := url.ParseQuery(newUrlQuery.RawQuery)
	require.Nil(t, err)
	hash := queryString.Get("test")
	require.NotNil(t, hash)

	// Make sure another call with same params generates a different hash
	anotherUrl := addCacheBustParam(target, paramName, salt)
	require.NotEqual(t, newUrl, anotherUrl)
}
