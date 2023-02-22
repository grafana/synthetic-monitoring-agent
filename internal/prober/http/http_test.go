package http

import (
	"context"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/http/testserver"
	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
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
				config: getDefaultModule().getConfigModule(),
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
		"headers": {
			input: sm.Check{
				Target: "www.grafana.com",
				Settings: sm.CheckSettings{
					Http: &sm.HttpSettings{
						Headers: []string{
							"uSeR-aGeNt: test-user-agent",
							"some-header: some-value",
						},
					},
				},
			},
			expected: Prober{
				config: getDefaultModule().
					addHttpHeader("uSeR-aGeNt", "test-user-agent").
					addHttpHeader("some-header", "some-value").
					getConfigModule(),
			},
			ExpectError: false,
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

func TestProbe(t *testing.T) {
	srv := testserver.New(testserver.Config{})
	defer srv.Close()

	testcases := map[string]struct {
		srvSettings       testserver.Settings
		settings          sm.HttpSettings
		expectFailure     bool
		expectedRedirects int
	}{
		"status 200 GET": {
			srvSettings: testserver.Settings{
				Method: "GET",
				Status: 200,
			},
			settings: sm.HttpSettings{
				Method: sm.HttpMethod_GET,
			},
		},
		"status 200 CONNECT": {
			srvSettings: testserver.Settings{
				Method: "CONNECT",
				Status: 200,
			},
			settings: sm.HttpSettings{
				Method: sm.HttpMethod_CONNECT,
			},
		},
		"status 200 DELETE": {
			srvSettings: testserver.Settings{
				Method: "DELETE",
				Status: 200,
			},
			settings: sm.HttpSettings{
				Method: sm.HttpMethod_DELETE,
			},
		},
		"status 200 HEAD": {
			srvSettings: testserver.Settings{
				Method: "HEAD",
				Status: 200,
			},
			settings: sm.HttpSettings{
				Method: sm.HttpMethod_HEAD,
			},
		},
		"status 200 OPTIONS": {
			srvSettings: testserver.Settings{
				Method: "OPTIONS",
				Status: 200,
			},
			settings: sm.HttpSettings{
				Method: sm.HttpMethod_OPTIONS,
			},
		},
		"status 200 POST": {
			srvSettings: testserver.Settings{
				Method: "POST",
				Status: 200,
			},
			settings: sm.HttpSettings{
				Method: sm.HttpMethod_POST,
			},
		},
		"status 200 PUT": {
			srvSettings: testserver.Settings{
				Method: "PUT",
				Status: 200,
			},
			settings: sm.HttpSettings{
				Method: sm.HttpMethod_PUT,
			},
		},
		"status 200 TRACE": {
			srvSettings: testserver.Settings{
				Method: "TRACE",
				Status: 200,
			},
			settings: sm.HttpSettings{
				Method: sm.HttpMethod_TRACE,
			},
		},
		"status 201": {
			srvSettings: testserver.Settings{
				Status: 201,
			},
			settings: sm.HttpSettings{},
		},
		"status 404": {
			srvSettings: testserver.Settings{
				Status: 404,
			},
			settings:      sm.HttpSettings{},
			expectFailure: true,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			target := tc.srvSettings.URL(srv.Listener.Addr().String())
			check := sm.Check{
				Id:        1,
				TenantId:  1,
				Frequency: 10000,
				Timeout:   1000,
				Enabled:   true,
				Settings: sm.CheckSettings{
					Http: &tc.settings,
				},
				Probes:           []int64{1},
				Target:           target,
				Job:              "test",
				BasicMetricsOnly: true,
			}

			t.Log(check.Target)

			ctx := context.Background()
			registry := prometheus.NewPedanticRegistry()
			zl := zerolog.Logger{}
			kl := log.NewLogfmtLogger(io.Discard)

			prober, err := NewProber(ctx, check, zl)
			require.NoError(t, err)
			require.Equal(t, tc.expectFailure, !prober.Probe(ctx, check.Target, registry, kl))

			mfs, err := registry.Gather()
			require.NoError(t, err)
			require.NotEmpty(t, mfs)
			foundMetrics := 0
			for _, mf := range mfs {
				require.NotNil(t, mf)
				switch name := mf.GetName(); name {
				case "probe_http_status_code":
					require.Equal(t, float64(tc.srvSettings.Status), getGaugeValue(t, mf))
					foundMetrics++

				case "probe_http_redirects":
					require.Equal(t, float64(tc.expectedRedirects), getGaugeValue(t, mf))
					foundMetrics++

				case "probe_http_version":
					require.Equal(t, 1.1, getGaugeValue(t, mf))
					foundMetrics++
				}
			}
			require.Equal(t, 3, foundMetrics)
		})
	}
}

func getGaugeValue(t *testing.T, mf *dto.MetricFamily) float64 {
	metric := mf.GetMetric()
	require.Len(t, metric, 1)
	g := metric[0].GetGauge()
	require.NotNil(t, g)
	return g.GetValue()
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
			input:    sm.HttpSettings{},
			expected: getDefaultModule().getConfigModule(),
		},
		"partial-settings": {
			input: sm.HttpSettings{
				ValidStatusCodes: []int32{200, 201},
				Method:           5,
				Body:             "This is a body",
			},
			expected: getDefaultModule().
				addHttpValidStatusCodes(200).
				addHttpValidStatusCodes(201).
				setHttpMethod("POST").
				setHttpBody("This is a body").
				getConfigModule(),
		},
		"proxy-settings": {
			input: sm.HttpSettings{
				ProxyURL:            "http://example.org/",
				ProxyConnectHeaders: []string{"h1: v1", "h2:v2"},
			},
			expected: getDefaultModule().
				setProxyUrl("http://example.org/").
				setProxyConnectHeaders(map[string]string{"h1": "v1", "h2": "v2"}).
				setSkipResolvePhaseWithProxy(true).
				getConfigModule(),
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

type testModule config.Module

func getDefaultModule() *testModule {
	testModule := testModule(config.Module{
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
				EnableHTTP2:     true,
			},
		},
	})

	return &testModule
}

func (m *testModule) getConfigModule() config.Module {
	return config.Module(*m)
}

func (m *testModule) addHttpHeader(key, value string) *testModule {
	if m.HTTP.Headers == nil {
		m.HTTP.Headers = make(map[string]string)
	}

	for k := range m.HTTP.Headers {
		if strings.EqualFold(k, key) {
			delete(m.HTTP.Headers, k)
		}
	}

	m.HTTP.Headers[key] = value

	return m
}

func (m *testModule) addHttpValidStatusCodes(code int) *testModule {
	m.HTTP.ValidStatusCodes = append(m.HTTP.ValidStatusCodes, code)
	return m
}

func (m *testModule) setHttpMethod(method string) *testModule {
	m.HTTP.Method = method
	return m
}

func (m *testModule) setHttpBody(body string) *testModule {
	m.HTTP.Body = body
	return m
}

func (m *testModule) setProxyUrl(u string) *testModule {
	var err error
	m.HTTP.HTTPClientConfig.ProxyURL.URL, err = url.Parse(u)
	if err != nil {
		panic(err)
	}
	return m
}

func (m *testModule) setProxyConnectHeaders(headers map[string]string) *testModule {
	m.HTTP.HTTPClientConfig.ProxyConnectHeader = make(httpConfig.Header)
	for k, v := range headers {
		m.HTTP.HTTPClientConfig.ProxyConnectHeader[k] = []httpConfig.Secret{httpConfig.Secret(v)}
	}
	return m
}

func (m *testModule) setSkipResolvePhaseWithProxy(value bool) *testModule {
	m.HTTP.SkipResolvePhaseWithProxy = value
	return m
}
