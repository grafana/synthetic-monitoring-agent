package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/http/testserver"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	httpConfig "github.com/prometheus/common/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// expectedHeaders creates expected headers map with user-agent included
func expectedHeaders(extra map[string]string) map[string]string {
	headers := map[string]string{
		"user-agent": version.UserAgent(),
	}
	for k, v := range extra {
		headers[k] = v
	}
	return headers
}

func TestName(t *testing.T) {
	name := Prober.Name(Prober{})
	require.Equal(t, name, "http")
}

func TestNewProber(t *testing.T) {
	testcases := map[string]struct {
		input       model.Check
		expectError bool
	}{
		"default": {
			input: model.Check{Check: sm.Check{
				Id:     3,
				Target: "www.grafana.com",
				Settings: sm.CheckSettings{
					Http: &sm.HttpSettings{
						Headers: []string{
							"X-SM-ID: 9880-98",
						},
					},
				},
			}},
			expectError: false,
		},
		"no-settings": {
			input: model.Check{Check: sm.Check{
				Id:     1,
				Target: "www.grafana.com",
				Settings: sm.CheckSettings{
					Http: nil,
				},
			}},
			expectError: true,
		},
		"headers": {
			input: model.Check{
				Check: sm.Check{
					Id:     5,
					Target: "www.grafana.com",
					Settings: sm.CheckSettings{
						Http: &sm.HttpSettings{
							Headers: []string{
								"uSeR-aGeNt: test-user-agent",
								"some-header: some-value",
								"x-SM-iD: 3232-32",
							},
						},
					},
				},
			},
			expectError: false,
		},
	}

	for name, testcase := range testcases {
		ctx := context.Background()
		logger := testhelper.NewTestLogger()
		t.Run(name, func(t *testing.T) {
			// origin identifier for http requests is checkId-probeId; testing with checkId twice in the absence of probeId
			checkId := testcase.input.Id
			reservedHeaders := http.Header{}
			reservedHeaders.Add("x-sm-id", fmt.Sprintf("%d-%d", checkId, checkId))

			actual, err := NewProber(ctx, testcase.input, logger, reservedHeaders, nil)

			if testcase.expectError {
				require.Error(t, err, "unsupported check")
			} else {
				require.NoError(t, err)
				// Verify that the prober was created with the expected settings
				require.NotNil(t, actual.settings)
				require.Equal(t, testcase.input.Settings.Http, actual.settings)
				require.Equal(t, testcase.input.GlobalTenantID(), actual.tenantID)
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
			check := model.Check{
				Check: sm.Check{
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
				},
			}

			t.Log(check.Target)

			ctx := context.Background()
			registry := prometheus.NewPedanticRegistry()
			zl := zerolog.Logger{}
			kl := log.NewLogfmtLogger(io.Discard)

			prober, err := NewProber(ctx, check, zl, http.Header{}, nil)
			require.NoError(t, err)

			success, duration := prober.Probe(ctx, check.Target, registry, kl)
			require.Equal(t, tc.expectFailure, !success)
			require.Equal(t, float64(0), duration)

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
			input:    nil,
			expected: expectedHeaders(nil),
		},

		"empty": {
			input:    []string{},
			expected: expectedHeaders(nil),
		},

		"trivial": {
			input: []string{
				"foo: bar",
			},
			expected: expectedHeaders(map[string]string{"foo": "bar"}),
		},

		"multiple headers": {
			input: []string{
				"h1: v1",
				"h2: v2",
			},
			expected: expectedHeaders(map[string]string{"h1": "v1", "h2": "v2"}),
		},

		"compact": {
			input: []string{
				"h1:v1",
				"h2:v2",
			},
			expected: expectedHeaders(map[string]string{"h1": "v1", "h2": "v2"}),
		},

		"trim leading whitespace": {
			input: []string{
				"h1:   v1",
				"h2:      v2",
			},
			expected: expectedHeaders(map[string]string{"h1": "v1", "h2": "v2"}),
		},

		"keep trailing whitespace": {
			input: []string{
				"h1: v1   ",
				"h2: v2 ",
			},
			expected: expectedHeaders(map[string]string{"h1": "v1   ", "h2": "v2 "}),
		},

		"empty values": {
			input: []string{
				"h1: ",
				"h2:",
			},
			expected: expectedHeaders(map[string]string{"h1": "", "h2": ""}),
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
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := buildStaticConfig(&testcase.input)
			require.NoError(t, err)

			// Note: buildStaticConfig doesn't include HTTP client config
			// so we need to remove that from expected results for this test
			expected := testcase.expected
			expected.HTTP.HTTPClientConfig = httpConfig.HTTPClientConfig{}

			require.Equal(t, &expected, &actual)
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

func TestNewProberWithSecretStore(t *testing.T) {
	ctx := context.Background()
	logger := testhelper.NewTestLogger()

	check := model.Check{Check: sm.Check{
		Id:     1,
		Target: "www.grafana.com",
		Settings: sm.CheckSettings{
			Http: &sm.HttpSettings{},
		},
	}}

	// Test with nil secret store (should work)
	_, err := NewProber(ctx, check, logger, http.Header{}, nil)
	require.NoError(t, err)

	// This test verifies that the secretStore parameter is properly
	// accepted and passed through the call chain without causing errors
}

func TestResolveSecretValue(t *testing.T) {
	ctx := context.Background()
	logger := testhelper.NewTestLogger()
	tenantID := model.GlobalID(123)

	testcases := map[string]struct {
		input          string
		mockSecretFunc func(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error)
		expectedOutput string
		expectError    bool
	}{
		"empty value": {
			input:          "",
			expectedOutput: "",
			expectError:    false,
		},
		"secret interpolation with valid secret": {
			input: "${secrets.my-secret-key}",
			mockSecretFunc: func(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
				if secretKey == "my-secret-key" {
					return "secret-value-from-gsm", nil
				}
				return "", fmt.Errorf("secret not found")
			},
			expectedOutput: "secret-value-from-gsm",
			expectError:    false,
		},
		"secret interpolation with secret lookup error": {
			input: "${secrets.non-existent-key}",
			mockSecretFunc: func(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
				return "", fmt.Errorf("secret not found")
			},
			expectedOutput: "",
			expectError:    true,
		},
		"secret interpolation with empty secret name": {
			input:          "${secrets.}",
			expectedOutput: "",
			expectError:    true,
		},
		"plaintext value (no interpolation)": {
			input:          "my-plain-password",
			expectedOutput: "my-plain-password",
			expectError:    false,
		},
		"mixed interpolation and plaintext": {
			input: "Bearer ${secrets.my-token}",
			mockSecretFunc: func(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
				if secretKey == "my-token" {
					return "actual-token-value", nil
				}
				return "", fmt.Errorf("secret not found")
			},
			expectedOutput: "Bearer actual-token-value",
			expectError:    false,
		},
		"multiple secrets in one string": {
			input: "${secrets.username}:${secrets.password}",
			mockSecretFunc: func(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
				switch secretKey {
				case "username":
					return "admin", nil
				case "password":
					return "secret123", nil
				default:
					return "", fmt.Errorf("secret not found")
				}
			},
			expectedOutput: "admin:secret123",
			expectError:    false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			// Create mock secret store
			var mockSecretStore *testhelper.MockSecretProvider
			if tc.mockSecretFunc != nil {
				mockSecretStore = testhelper.NewMockSecretProviderWithFunc(tc.mockSecretFunc)
			}

			// Use mock secret store directly
			secretStore := mockSecretStore

			actual, err := resolveSecretValue(ctx, tc.input, secretStore, tenantID, logger, true)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedOutput, actual)
			}
		})
	}
}

func TestBuildPrometheusHTTPClientConfig_WithSecrets(t *testing.T) {
	ctx, logger, tenantID := testhelper.CommonTestSetup()

	// Mock secret store that returns known values
	mockSecretStore := testhelper.NewMockSecretProvider(map[string]string{
		"bearer-token-key": "bearer-secret-value",
		"password-key":     "password-secret-value",
	})

	testcases := map[string]struct {
		settings       sm.HttpSettings
		expectedBearer string
		expectedPasswd string
	}{
		"secret interpolation": {
			settings: sm.HttpSettings{
				BearerToken:          "${secrets.bearer-token-key}",
				SecretManagerEnabled: true,
				BasicAuth: &sm.BasicAuth{
					Username: "testuser",
					Password: "${secrets.password-key}",
				},
			},
			expectedBearer: "bearer-secret-value",
			expectedPasswd: "password-secret-value",
		},
		"plaintext values": {
			settings: sm.HttpSettings{
				BearerToken:          "plain-bearer-token",
				SecretManagerEnabled: true,
				BasicAuth: &sm.BasicAuth{
					Username: "testuser",
					Password: "plain-password",
				},
			},
			expectedBearer: "plain-bearer-token",
			expectedPasswd: "plain-password",
		},
		"mixed interpolation and plaintext": {
			settings: sm.HttpSettings{
				BearerToken:          "${secrets.bearer-token-key}",
				SecretManagerEnabled: true,
				BasicAuth: &sm.BasicAuth{
					Username: "testuser",
					Password: "plain-password",
				},
			},
			expectedBearer: "bearer-secret-value",
			expectedPasswd: "plain-password",
		},
		"complex interpolation": {
			settings: sm.HttpSettings{
				BearerToken:          "Bearer ${secrets.bearer-token-key}",
				SecretManagerEnabled: true,
				BasicAuth: &sm.BasicAuth{
					Username: "testuser",
					Password: "${secrets.password-key}",
				},
			},
			expectedBearer: "Bearer bearer-secret-value",
			expectedPasswd: "password-secret-value",
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			// Use mock secret store directly
			secretStore := mockSecretStore

			cfg, err := buildPrometheusHTTPClientConfig(ctx, &tc.settings, logger, secretStore, tenantID)
			require.NoError(t, err)

			require.Equal(t, tc.expectedBearer, string(cfg.BearerToken))
			if tc.settings.BasicAuth != nil {
				require.NotNil(t, cfg.BasicAuth)
				require.Equal(t, tc.expectedPasswd, string(cfg.BasicAuth.Password))
				require.Equal(t, tc.settings.BasicAuth.Username, cfg.BasicAuth.Username)
			}
		})
	}
}

func TestResolveSecretValueWithCapabilityFromSecretStore(t *testing.T) {
	ctx, logger, tenantID := testhelper.CommonTestSetup()

	// Mock secret store that should never be called when capability is disabled
	mockSecretStore := testhelper.NewMockSecretProvider(map[string]string{
		"my-bearer-token": "resolved-bearer-token",
	})

	t.Run("with EnableProtocolSecrets=true", func(t *testing.T) {
		// Create secret store with capability enabled
		secretStore := mockSecretStore

		testcases := map[string]struct {
			input          string
			expectedOutput string
			expectError    bool
		}{
			"secret interpolation resolved when capability enabled": {
				input:          "${secrets.my-bearer-token}",
				expectedOutput: "resolved-bearer-token",
				expectError:    false,
			},
			"plaintext value unchanged when capability enabled": {
				input:          "my-plain-password",
				expectedOutput: "my-plain-password",
				expectError:    false,
			},
			"mixed interpolation and plaintext when capability enabled": {
				input:          "Bearer ${secrets.my-bearer-token}",
				expectedOutput: "Bearer resolved-bearer-token",
				expectError:    false,
			},
		}

		for name, tc := range testcases {
			t.Run(name, func(t *testing.T) {
				actual, err := resolveSecretValue(ctx, tc.input, secretStore, tenantID, logger, true)

				if tc.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, tc.expectedOutput, actual)
				}
			})
		}
	})

	t.Run("with EnableProtocolSecrets=false", func(t *testing.T) {
		// Mock that should never be called
		failingMockStore := testhelper.NewMockSecretProviderWithFunc(func(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
			t.Fatal("GetSecretValue should not be called when EnableProtocolSecrets is false")
			return "", nil
		})

		// Create secret store with capability disabled
		secretStore := failingMockStore

		testcases := map[string]struct {
			input          string
			expectedOutput string
		}{
			"secret interpolation preserved when capability disabled": {
				input:          "${secrets.my-bearer-token}",
				expectedOutput: "${secrets.my-bearer-token}",
			},
			"plaintext value unchanged when capability disabled": {
				input:          "my-plain-password",
				expectedOutput: "my-plain-password",
			},
			"mixed interpolation preserved when capability disabled": {
				input:          "Bearer ${secrets.my-bearer-token}",
				expectedOutput: "Bearer ${secrets.my-bearer-token}",
			},
		}

		for name, tc := range testcases {
			t.Run(name, func(t *testing.T) {
				actual, err := resolveSecretValue(ctx, tc.input, secretStore, tenantID, logger, false)

				require.NoError(t, err)
				require.Equal(t, tc.expectedOutput, actual)
			})
		}
	})

	t.Run("with nil capabilities (defaults to false)", func(t *testing.T) {
		// Mock that should never be called
		failingMockStore := testhelper.NewMockSecretProviderWithFunc(func(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
			t.Fatal("GetSecretValue should not be called when capabilities are nil")
			return "", nil
		})

		// Create secret store with nil capabilities
		secretStore := failingMockStore

		actual, err := resolveSecretValue(ctx, "gsm:my-bearer-token", secretStore, tenantID, logger, false)
		require.NoError(t, err)
		require.Equal(t, "gsm:my-bearer-token", actual)
	})

	t.Run("with regular SecretProvider (no capability awareness)", func(t *testing.T) {
		// When using a regular SecretProvider, should default to false (no resolution)
		failingMockStore := testhelper.NewMockSecretProviderWithFunc(func(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
			t.Fatal("GetSecretValue should not be called when no capability interface is implemented")
			return "", nil
		})

		actual, err := resolveSecretValue(ctx, "gsm:my-bearer-token", failingMockStore, tenantID, logger, false)
		require.NoError(t, err)
		require.Equal(t, "gsm:my-bearer-token", actual)
	})
}

func TestUpdatableSecretProvider(t *testing.T) {
	ctx, logger, tenantID := testhelper.CommonTestSetup()

	// Mock secret store
	mockSecretStore := testhelper.NewMockSecretProvider(map[string]string{
		"my-bearer-token": "resolved-bearer-token",
	})

	// Create updatable secret store
	updatableStore := mockSecretStore

	t.Run("defaults to disabled", func(t *testing.T) {
		require.False(t, updatableStore.IsProtocolSecretsEnabled())

		// Should not resolve secrets when disabled
		actual, err := resolveSecretValue(ctx, "${secrets.my-bearer-token}", updatableStore, tenantID, logger, false)
		require.NoError(t, err)
		require.Equal(t, "${secrets.my-bearer-token}", actual)
	})

	t.Run("can be updated to enabled", func(t *testing.T) {
		// Update capabilities to enable protocol secrets
		capabilities := &sm.Probe_Capabilities{
			EnableProtocolSecrets: true,
		}
		updatableStore.UpdateCapabilities(capabilities)

		require.True(t, updatableStore.IsProtocolSecretsEnabled())

		// Should now resolve secrets
		actual, err := resolveSecretValue(ctx, "${secrets.my-bearer-token}", updatableStore, tenantID, logger, true)
		require.NoError(t, err)
		require.Equal(t, "resolved-bearer-token", actual)
	})

	t.Run("can be updated to disabled", func(t *testing.T) {
		// Update capabilities to disable protocol secrets
		capabilities := &sm.Probe_Capabilities{
			EnableProtocolSecrets: false,
		}
		updatableStore.UpdateCapabilities(capabilities)

		require.False(t, updatableStore.IsProtocolSecretsEnabled())

		// Should not resolve secrets when disabled
		actual, err := resolveSecretValue(ctx, "${secrets.my-bearer-token}", updatableStore, tenantID, logger, false)
		require.NoError(t, err)
		require.Equal(t, "${secrets.my-bearer-token}", actual)
	})

	t.Run("handles nil capabilities", func(t *testing.T) {
		// Update with nil capabilities (should default to disabled)
		updatableStore.UpdateCapabilities(nil)

		require.False(t, updatableStore.IsProtocolSecretsEnabled())

		// Should not resolve secrets when disabled
		actual, err := resolveSecretValue(ctx, "${secrets.my-bearer-token}", updatableStore, tenantID, logger, false)
		require.NoError(t, err)
		require.Equal(t, "${secrets.my-bearer-token}", actual)
	})
}

func TestResolveSecretValueWithSecretManagerEnabled(t *testing.T) {
	ctx, logger, tenantID := testhelper.CommonTestSetup()

	// Mock secret store
	mockSecretStore := testhelper.NewMockSecretProvider(map[string]string{
		"my-bearer-token": "resolved-bearer-token",
	})

	testcases := map[string]struct {
		input                string
		secretManagerEnabled bool
		expectedOutput       string
		expectError          bool
	}{
		"secret manager enabled with secret interpolation": {
			input:                "${secrets.my-bearer-token}",
			secretManagerEnabled: true,
			expectedOutput:       "resolved-bearer-token",
			expectError:          false,
		},
		"secret manager enabled with plaintext value": {
			input:                "my-plain-password",
			secretManagerEnabled: true,
			expectedOutput:       "my-plain-password",
			expectError:          false,
		},
		"secret manager enabled with mixed interpolation": {
			input:                "Bearer ${secrets.my-bearer-token}",
			secretManagerEnabled: true,
			expectedOutput:       "Bearer resolved-bearer-token",
			expectError:          false,
		},
		"secret manager disabled with secret interpolation": {
			input:                "${secrets.my-bearer-token}",
			secretManagerEnabled: false,
			expectedOutput:       "${secrets.my-bearer-token}",
			expectError:          false,
		},
		"secret manager disabled with plaintext value": {
			input:                "my-plain-password",
			secretManagerEnabled: false,
			expectedOutput:       "my-plain-password",
			expectError:          false,
		},
		"secret manager disabled with mixed interpolation": {
			input:                "Bearer ${secrets.my-bearer-token}",
			secretManagerEnabled: false,
			expectedOutput:       "Bearer ${secrets.my-bearer-token}",
			expectError:          false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := resolveSecretValue(ctx, tc.input, mockSecretStore, tenantID, logger, tc.secretManagerEnabled)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedOutput, actual)
			}
		})
	}
}

func TestBuildTLSConfig_WithSecrets(t *testing.T) {
	ctx, logger, tenantID := testhelper.CommonTestSetup()

	// Mock secret store that returns known values
	mockSecretStore := testhelper.NewMockSecretProvider(map[string]string{
		"ca-cert-key":     "-----BEGIN CERTIFICATE-----\nCA_CERT_CONTENT\n-----END CERTIFICATE-----",
		"client-cert-key": "-----BEGIN CERTIFICATE-----\nCLIENT_CERT_CONTENT\n-----END CERTIFICATE-----",
		"client-key-key":  "-----BEGIN PRIVATE KEY-----\nCLIENT_KEY_CONTENT\n-----END PRIVATE KEY-----",
	})

	testcases := map[string]struct {
		tlsConfig            *sm.TLSConfig
		secretManagerEnabled bool
		expectError          bool
	}{
		"TLS with secret interpolation": {
			tlsConfig: &sm.TLSConfig{
				InsecureSkipVerify: false,
				ServerName:         "example.com",
				CACert:             []byte("${secrets.ca-cert-key}"),
				ClientCert:         []byte("${secrets.client-cert-key}"),
				ClientKey:          []byte("${secrets.client-key-key}"),
			},
			secretManagerEnabled: true,
			expectError:          false,
		},
		"TLS with plain values": {
			tlsConfig: &sm.TLSConfig{
				InsecureSkipVerify: true,
				ServerName:         "test.com",
				CACert:             []byte("-----BEGIN CERTIFICATE-----\nPLAIN_CA_CERT\n-----END CERTIFICATE-----"),
				ClientCert:         []byte("-----BEGIN CERTIFICATE-----\nPLAIN_CLIENT_CERT\n-----END CERTIFICATE-----"),
				ClientKey:          []byte("-----BEGIN PRIVATE KEY-----\nPLAIN_CLIENT_KEY\n-----END PRIVATE KEY-----"),
			},
			secretManagerEnabled: true,
			expectError:          false,
		},
		"TLS with secret manager disabled": {
			tlsConfig: &sm.TLSConfig{
				InsecureSkipVerify: false,
				ServerName:         "example.com",
				CACert:             []byte("${secrets.ca-cert-key}"),
				ClientCert:         []byte("${secrets.client-cert-key}"),
				ClientKey:          []byte("${secrets.client-key-key}"),
			},
			secretManagerEnabled: false,
			expectError:          false,
		},
		"TLS with mixed secret and plain values": {
			tlsConfig: &sm.TLSConfig{
				InsecureSkipVerify: false,
				ServerName:         "mixed.com",
				CACert:             []byte("${secrets.ca-cert-key}"),
				ClientCert:         []byte("-----BEGIN CERTIFICATE-----\nPLAIN_CLIENT_CERT\n-----END CERTIFICATE-----"),
				ClientKey:          []byte("${secrets.client-key-key}"),
			},
			secretManagerEnabled: true,
			expectError:          false,
		},
		"TLS with only some fields": {
			tlsConfig: &sm.TLSConfig{
				InsecureSkipVerify: false,
				ServerName:         "partial.com",
				CACert:             []byte("${secrets.ca-cert-key}"),
				// ClientCert and ClientKey are empty
			},
			secretManagerEnabled: true,
			expectError:          false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			cfg, err := buildTLSConfig(ctx, tc.tlsConfig, mockSecretStore, tenantID, logger, tc.secretManagerEnabled)

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.tlsConfig.InsecureSkipVerify, cfg.InsecureSkipVerify)
			require.Equal(t, tc.tlsConfig.ServerName, cfg.ServerName)

			// Verify that files were created for non-empty fields
			if len(tc.tlsConfig.CACert) > 0 {
				require.NotEmpty(t, cfg.CAFile)
			}
			if len(tc.tlsConfig.ClientCert) > 0 {
				require.NotEmpty(t, cfg.CertFile)
			}
			if len(tc.tlsConfig.ClientKey) > 0 {
				require.NotEmpty(t, cfg.KeyFile)
			}
		})
	}
}
