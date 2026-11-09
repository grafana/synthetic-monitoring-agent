package http

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/interpolation"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// Secrets shared by these tests. Each value differs from its reference, so a
// resolved value never looks like a reference sent as text.
const (
	wireTokenName  = "api-token"
	wireTokenRef   = "${secrets.api-token}"
	wireTokenValue = "resolved-token-value"

	wirePasswordName  = "api-password"
	wirePasswordRef   = "${secrets.api-password}"
	wirePasswordValue = "resolved-password-value"

	wireUsername = "check-user"
)

// capturedRequest is the part of an inbound request these tests assert on.
type capturedRequest struct {
	header http.Header
	host   string
}

// requestRecorder is a probe target that keeps the requests it receives, so a
// test can check the exact headers the prober sent. The testserver package drops
// request headers, so it does not work here.
type requestRecorder struct {
	*httptest.Server

	mu       sync.Mutex
	captured []capturedRequest
}

func newRequestRecorder(t *testing.T) *requestRecorder {
	t.Helper()

	rec := &requestRecorder{}

	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec.mu.Lock()
		rec.captured = append(rec.captured, capturedRequest{header: req.Header.Clone(), host: req.Host})
		rec.mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))

	t.Cleanup(rec.Close)

	return rec
}

func (r *requestRecorder) requests() []capturedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]capturedRequest(nil), r.captured...)
}

func basicAuthHeader(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func wireCheck(target string, settings *sm.HttpSettings) model.Check {
	return model.Check{
		Check: sm.Check{
			Id:               1,
			TenantId:         1,
			Frequency:        10000,
			Timeout:          5000,
			Enabled:          true,
			Probes:           []int64{1},
			Target:           target,
			Job:              "test",
			BasicMetricsOnly: true,
			Settings:         sm.CheckSettings{Http: settings},
		},
	}
}

// TestInterpolationOnTheWire runs a full probe against a recording target and
// checks the credentials the target received, for each shape of value.
func TestInterpolationOnTheWire(t *testing.T) {
	secretStore := testhelper.NewMockSecretProvider(map[string]string{
		wireTokenName:    wireTokenValue,
		wirePasswordName: wirePasswordValue,
	})

	failingStore := testhelper.NewMockSecretProviderWithFunc(
		func(context.Context, model.GlobalID, string) (string, error) {
			return "", errors.New("no secret store configured for tenant 1")
		},
	)

	testcases := map[string]struct {
		settings sm.HttpSettings
		// secretStore overrides the shared store for this case only.
		secretStore       secrets.SecretProvider
		reservedHeaders   http.Header
		wantAuthorization string
		wantHeader        map[string]string
		wantHost          string
		wantFailure       bool
		// wantLogContains is matched against the agent log: the error on failure, or the secret name on success.
		wantLogContains    string
		wantLogNotContains string
		// wantCheckLogContains is matched against the check log
		wantCheckLogContains string
		// allowValueInCheckLogs allows a resolved value in the check logs (only host header)
		allowValueInCheckLogs bool
	}{
		"plaintext bearer token": {
			settings: sm.HttpSettings{
				BearerToken: "plain-token",
			},
			wantAuthorization: "Bearer plain-token",
		},

		"secret reference in bearer token": {
			settings: sm.HttpSettings{
				BearerToken: wireTokenRef,
			},
			wantAuthorization: "Bearer " + wireTokenValue,
			wantLogContains:   wireTokenName,
		},

		"plaintext basic auth password": {
			settings: sm.HttpSettings{
				BasicAuth: &sm.BasicAuth{
					Username: wireUsername,
					Password: "plain-password",
				},
			},
			wantAuthorization: basicAuthHeader(wireUsername, "plain-password"),
		},

		"secret reference in basic auth password": {
			settings: sm.HttpSettings{
				BasicAuth: &sm.BasicAuth{
					Username: wireUsername,
					Password: wirePasswordRef,
				},
			},
			wantAuthorization: basicAuthHeader(wireUsername, wirePasswordValue),
			wantLogContains:   wirePasswordName,
		},

		"secret reference surrounded by literal text": {
			settings: sm.HttpSettings{
				BasicAuth: &sm.BasicAuth{
					Username: wireUsername,
					Password: "prefix-" + wirePasswordRef + "-suffix",
				},
			},
			wantAuthorization: basicAuthHeader(wireUsername, "prefix-"+wirePasswordValue+"-suffix"),
		},

		"two secret references in one value": {
			settings: sm.HttpSettings{
				BasicAuth: &sm.BasicAuth{
					Username: wireUsername,
					Password: wireTokenRef + ":" + wirePasswordRef,
				},
			},
			wantAuthorization: basicAuthHeader(wireUsername, wireTokenValue+":"+wirePasswordValue),
		},

		// A stray "${" stays as text and does not stop the reference after it.
		"secret reference after an unterminated placeholder": {
			settings: sm.HttpSettings{
				BearerToken: "prefix ${ " + wireTokenRef + " suffix",
			},
			wantAuthorization: "Bearer prefix ${ " + wireTokenValue + " suffix",
		},

		"secret reference nested in an unterminated placeholder": {
			settings: sm.HttpSettings{
				BearerToken: "${" + wireTokenRef,
			},
			wantAuthorization: "Bearer ${" + wireTokenValue,
		},

		// Only ${secrets.name} is resolved. A bare ${name} stays as is.
		"variable syntax is not interpolated": {
			settings: sm.HttpSettings{
				BearerToken: "${api-token}",
			},
			wantAuthorization: "Bearer ${api-token}",
		},

		// A "$" outside "${...}" is plain text, and so is "$$".
		"dollar without a placeholder": {
			settings: sm.HttpSettings{
				BearerToken: "cost: $5.00",
			},
			wantAuthorization: "Bearer cost: $5.00",
		},

		"unbraced dollar name": {
			settings: sm.HttpSettings{
				BearerToken: "$api-token",
			},
			wantAuthorization: "Bearer $api-token",
		},

		"doubled dollar": {
			settings: sm.HttpSettings{
				BearerToken: "$${api-token}",
			},
			wantAuthorization: "Bearer $${api-token}",
		},

		"secret reference in a custom header": {
			settings: sm.HttpSettings{
				Headers: []string{"X-Api-Key: " + wireTokenRef},
			},
			wantHeader:      map[string]string{"X-Api-Key": wireTokenValue},
			wantLogContains: wireTokenName,
		},

		"secret reference with text in a custom header": {
			settings: sm.HttpSettings{
				Headers: []string{"Authorization: ApiKey " + wireTokenRef},
			},
			wantAuthorization: "ApiKey " + wireTokenValue,
			wantLogContains:   wireTokenName,
		},

		"two secret references in a custom header": {
			settings: sm.HttpSettings{
				Headers: []string{"X-Credentials: " + wireTokenRef + ":" + wirePasswordRef},
			},
			wantHeader:      map[string]string{"X-Credentials": wireTokenValue + ":" + wirePasswordValue},
			wantLogContains: wirePasswordName,
		},

		"secret references in two custom headers next to a plaintext one": {
			settings: sm.HttpSettings{
				Headers: []string{
					"X-Api-Key: " + wireTokenRef,
					"X-Plain: plain-value",
					"X-Api-Password: " + wirePasswordRef,
				},
			},
			wantHeader: map[string]string{
				"X-Api-Key":      wireTokenValue,
				"X-Plain":        "plain-value",
				"X-Api-Password": wirePasswordValue,
			},
			wantLogContains: wirePasswordName,
		},

		// Reserved headers never resolved
		"secret reference in a reserved header": {
			settings: sm.HttpSettings{
				Headers: []string{"x-sm-id: " + wireTokenRef},
			},
			reservedHeaders:    http.Header{"X-Sm-Id": {"1-1"}},
			wantHeader:         map[string]string{"X-Sm-Id": "1-1"},
			wantLogNotContains: wireTokenName,
		},

		// A header name is never resolved.
		"secret reference in a custom header name": {
			settings: sm.HttpSettings{
				Headers: []string{wireTokenRef + ": plain-value"},
			},
			wantFailure:          true,
			wantLogNotContains:   wireTokenName,
			wantCheckLogContains: "invalid header field name",
		},

		"secret reference in the User-Agent header": {
			settings: sm.HttpSettings{
				Headers: []string{"User-Agent: " + wireTokenRef},
			},
			wantHeader:      map[string]string{"User-Agent": wireTokenValue},
			wantLogContains: wireTokenName,
		},

		"secret reference in the Host header": {
			settings: sm.HttpSettings{
				Headers: []string{"Host: " + wireTokenRef},
			},
			wantHost:              wireTokenValue,
			wantLogContains:       wireTokenName,
			wantCheckLogContains:  wireTokenValue,
			allowValueInCheckLogs: true,
		},

		// Invalid value in the header (API stores it with no trim or modification)
		"secret value ending in a newline in a custom header": {
			settings: sm.HttpSettings{
				Headers: []string{"X-Api-Key: " + wireTokenRef},
			},
			secretStore: testhelper.NewMockSecretProvider(map[string]string{
				wireTokenName: wireTokenValue + "\n",
			}),
			wantFailure:          true,
			wantCheckLogContains: `invalid header field value for \"X-Api-Key\"`,
		},

		// The error names the header and the secret
		"unknown secret in a custom header": {
			settings: sm.HttpSettings{
				Headers: []string{"X-Api-Key: ${secrets.does-not-exist}"},
			},
			wantFailure:     true,
			wantLogContains: `failed to resolve header \"X-Api-Key\": failed to get secret 'does-not-exist'`,
		},

		"invalid secret name in a custom header": {
			settings: sm.HttpSettings{
				Headers: []string{"X-Api-Key: ${secrets.My_Token}"},
			},
			wantFailure:     true,
			wantLogContains: `failed to resolve header \"X-Api-Key\": invalid secret name`,
		},

		"invalid secret name": {
			settings: sm.HttpSettings{
				BearerToken: "${secrets.My_Token}",
			},
			wantFailure:     true,
			wantLogContains: "invalid secret name",
		},

		"empty secret name": {
			settings: sm.HttpSettings{
				BearerToken: "${secrets.}",
			},
			wantFailure:     true,
			wantLogContains: "invalid secret name",
		},

		"unknown secret": {
			settings: sm.HttpSettings{
				BearerToken: "${secrets.does-not-exist}",
			},
			wantFailure:     true,
			wantLogContains: "secret not found",
		},

		"secret store unavailable": {
			settings: sm.HttpSettings{
				BearerToken: wireTokenRef,
			},
			secretStore:     failingStore,
			wantFailure:     true,
			wantLogContains: "no secret store configured",
		},

		// The error must name the field that failed.
		"unknown secret in basic auth password": {
			settings: sm.HttpSettings{
				BasicAuth: &sm.BasicAuth{
					Username: wireUsername,
					Password: "${secrets.does-not-exist}",
				},
			},
			wantFailure:     true,
			wantLogContains: "failed to resolve basic auth password",
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := testhelper.Context(context.Background(), t)
			t.Cleanup(cancel)

			target := newRequestRecorder(t)

			store := tc.secretStore
			if store == nil {
				store = secretStore
			}

			settings := tc.settings

			check := wireCheck(target.URL, &settings)

			// Resolution errors go to the logger passed to NewProber, not to the
			// one passed to Probe.
			var logs testhelper.LogBuffer

			reservedHeaders := tc.reservedHeaders
			if reservedHeaders == nil {
				reservedHeaders = http.Header{}
			}

			prober, err := NewProber(ctx, check, zerolog.New(&logs), reservedHeaders, store)
			require.NoError(t, err)

			var checkLogs testhelper.LogBuffer

			success, _ := prober.Probe(
				ctx,
				check.Target,
				prometheus.NewPedanticRegistry(),
				log.NewLogfmtLogger(&checkLogs),
				"test-execution-id",
			)

			requests := target.requests()

			if tc.wantLogContains != "" {
				require.Contains(t, logs.String(), tc.wantLogContains)
			}

			if tc.wantCheckLogContains != "" {
				require.Contains(t, checkLogs.String(), tc.wantCheckLogContains)
			}

			if tc.wantLogNotContains != "" {
				require.NotContains(t, logs.String(), tc.wantLogNotContains)
			}

			// Secret values must never reach the logs.
			for _, value := range []string{wireTokenValue, wirePasswordValue} {
				require.NotContains(t, logs.String(), value)

				if !tc.allowValueInCheckLogs {
					require.NotContains(t, checkLogs.String(), value)
				}
			}

			if tc.wantFailure {
				require.False(t, success)
				require.Empty(t, requests, "nothing may reach the target when the run fails")

				return
			}

			require.True(t, success)
			require.Len(t, requests, 1)

			// Exact equality, never Contains: "Bearer ${secrets.api-token}" and
			// "Bearer resolved-token-value" both contain "Bearer".
			require.Equal(t, tc.wantAuthorization, requests[0].header.Get("Authorization"))

			for header, value := range tc.wantHeader {
				require.Equal(t, value, requests[0].header.Get(header))
			}

			if tc.wantHost != "" {
				require.Equal(t, tc.wantHost, requests[0].host)
			}
		})
	}
}

// TestCustomHeaderSecretRotation runs one prober twice while the secret
// changes in between.
func TestCustomHeaderSecretRotation(t *testing.T) {
	ctx, cancel := testhelper.Context(context.Background(), t)
	t.Cleanup(cancel)

	target := newRequestRecorder(t)

	current := "first-value"
	store := testhelper.NewMockSecretProviderWithFunc(
		func(context.Context, model.GlobalID, string) (string, error) {
			return current, nil
		},
	)

	check := wireCheck(target.URL, &sm.HttpSettings{Headers: []string{"X-Api-Key: " + wireTokenRef}})

	var logs, checkLogs testhelper.LogBuffer

	prober, err := NewProber(ctx, check, zerolog.New(&logs), http.Header{}, store)
	require.NoError(t, err)

	values := []string{"first-value", "rotated-value"}

	for _, value := range values {
		current = value

		success, _ := prober.Probe(ctx, check.Target, prometheus.NewPedanticRegistry(), log.NewLogfmtLogger(&checkLogs), "test-execution-id")
		require.True(t, success)
	}

	require.Contains(t, logs.String(), wireTokenName)

	for _, value := range values {
		require.NotContains(t, logs.String(), value)
		require.NotContains(t, checkLogs.String(), value)
	}

	requests := target.requests()
	require.Len(t, requests, 2)
	require.Equal(t, "first-value", requests[0].header.Get("X-Api-Key"))
	require.Equal(t, "rotated-value", requests[1].header.Get("X-Api-Key"))
}

// TLS fixtures. Only the bytes matter, so they are not real certificates.
const (
	wireCACertPEM     = "-----BEGIN CERTIFICATE-----\nCA_CERT_CONTENT\n-----END CERTIFICATE-----\n"
	wireClientCertPEM = "-----BEGIN CERTIFICATE-----\nCLIENT_CERT_CONTENT\n-----END CERTIFICATE-----\n"
	wireClientKeyPEM  = "-----BEGIN PRIVATE KEY-----\nCLIENT_KEY_CONTENT\n-----END PRIVATE KEY-----\n"
)

// TestInterpolationInTLSMaterial checks what lands in the temp files the TLS
// config points to. Checking on the wire would need a full mTLS server for the
// same result.
func TestInterpolationInTLSMaterial(t *testing.T) {
	secretStore := testhelper.NewMockSecretProvider(map[string]string{
		"ca-cert":     wireCACertPEM,
		"client-cert": wireClientCertPEM,
		"client-key":  wireClientKeyPEM,
	})

	testcases := map[string]struct {
		settings       sm.HttpSettings
		wantCACert     string
		wantClientCert string
		wantClientKey  string
		wantError      string
	}{
		"secret reference in CA cert": {
			settings: sm.HttpSettings{
				TlsConfig: &sm.TLSConfig{
					CACert: []byte("${secrets.ca-cert}"),
				},
			},
			wantCACert: wireCACertPEM,
		},

		"plaintext PEM in CA cert": {
			settings: sm.HttpSettings{
				TlsConfig: &sm.TLSConfig{
					CACert: []byte(wireCACertPEM),
				},
			},
			wantCACert: wireCACertPEM,
		},

		"secret references in client cert and key": {
			settings: sm.HttpSettings{
				TlsConfig: &sm.TLSConfig{
					ClientCert: []byte("${secrets.client-cert}"),
					ClientKey:  []byte("${secrets.client-key}"),
				},
			},
			wantClientCert: wireClientCertPEM,
			wantClientKey:  wireClientKeyPEM,
		},

		"invalid secret name in CA cert": {
			settings: sm.HttpSettings{
				TlsConfig: &sm.TLSConfig{
					CACert: []byte("${secrets.My_CA}"),
				},
			},
			wantError: "invalid secret name",
		},

		"unknown secret in CA cert": {
			settings: sm.HttpSettings{
				TlsConfig: &sm.TLSConfig{
					CACert: []byte("${secrets.does-not-exist}"),
				},
			},
			wantError: "secret not found",
		},

		// The error must name the field that failed.
		"unknown secret in client cert": {
			settings: sm.HttpSettings{
				TlsConfig: &sm.TLSConfig{
					ClientCert: []byte("${secrets.does-not-exist}"),
				},
			},
			wantError: "failed to resolve client cert",
		},

		"unknown secret in client key": {
			settings: sm.HttpSettings{
				TlsConfig: &sm.TLSConfig{
					ClientKey: []byte("${secrets.does-not-exist}"),
				},
			},
			wantError: "failed to resolve client key",
		},
	}

	requireFileContents := func(t *testing.T, path, want string) {
		t.Helper()

		if want == "" {
			require.Empty(t, path)

			return
		}

		require.NotEmpty(t, path)

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, want, string(got))
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := testhelper.Context(context.Background(), t)
			t.Cleanup(cancel)

			settings := tc.settings

			var logs testhelper.LogBuffer

			logger := zerolog.New(&logs)

			cfg, err := buildPrometheusHTTPClientConfig(
				ctx,
				&settings,
				logger,
				interpolation.NewResolver(secretStore, model.GlobalID(1), logger),
			)

			// TLS material must never reach the logs. Match the PEM body, not the
			// whole fixture: zerolog escapes the newlines, so the whole fixture
			// would never match even when it leaks.
			require.NotContains(t, logs.String(), "CA_CERT_CONTENT")
			require.NotContains(t, logs.String(), "CLIENT_CERT_CONTENT")
			require.NotContains(t, logs.String(), "CLIENT_KEY_CONTENT")

			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)

				return
			}

			require.NoError(t, err)

			requireFileContents(t, cfg.TLSConfig.CAFile, tc.wantCACert)
			requireFileContents(t, cfg.TLSConfig.CertFile, tc.wantClientCert)
			requireFileContents(t, cfg.TLSConfig.KeyFile, tc.wantClientKey)
		})
	}
}
