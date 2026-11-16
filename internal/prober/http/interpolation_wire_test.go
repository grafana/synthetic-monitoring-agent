package http

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
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
		rec.captured = append(rec.captured, capturedRequest{header: req.Header.Clone()})
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

// TestInterpolationOnTheWire runs a full probe against a recording target and
// checks the credentials the target received, for each shape of value.
func TestInterpolationOnTheWire(t *testing.T) {
	secretStore := testhelper.NewMockSecretProvider(map[string]string{
		wireTokenName:    wireTokenValue,
		wirePasswordName: wirePasswordValue,
	})

	failingStore := testhelper.NewMockSecretProviderWithFunc(
		func(context.Context, model.GlobalID, string) (string, error) {
			return "", secrets.ErrNoSecretStore
		},
	)

	testcases := map[string]struct {
		settings sm.HttpSettings
		// secretStore overrides the shared store for this case only.
		secretStore       secrets.SecretProvider
		wantAuthorization string
		wantHeader        map[string]string
		wantFailure       bool
		// wantLogContains is the error on failure, or the secret name on success.
		wantLogContains string
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

		// Custom headers are never resolved, so the reference is sent as text.
		"secret reference in a custom header": {
			settings: sm.HttpSettings{
				Headers: []string{"X-Api-Key: " + wireTokenRef},
			},
			wantHeader: map[string]string{"X-Api-Key": wireTokenRef},
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
			wantLogContains: secrets.ErrSecretNotFound.Error(),
		},

		"secret store unavailable": {
			settings: sm.HttpSettings{
				BearerToken: wireTokenRef,
			},
			secretStore:     failingStore,
			wantFailure:     true,
			wantLogContains: secrets.ErrNoSecretStore.Error(),
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

			check := model.Check{
				Check: sm.Check{
					Id:               1,
					TenantId:         1,
					Frequency:        10000,
					Timeout:          5000,
					Enabled:          true,
					Probes:           []int64{1},
					Target:           target.URL,
					Job:              "test",
					BasicMetricsOnly: true,
					Settings: sm.CheckSettings{
						Http: &settings,
					},
				},
			}

			// The agent's logger!
			// See `TestSecretFailureReachesTheCheckLogs` for the check owner log tests.
			var agentLogger testhelper.LogBuffer

			prober, err := NewProber(ctx, check, zerolog.New(&agentLogger), http.Header{}, store)
			require.NoError(t, err)

			success, _ := prober.Probe(
				ctx,
				check.Target,
				prometheus.NewPedanticRegistry(),
				log.NewLogfmtLogger(io.Discard),
				"test-execution-id",
			)

			requests := target.requests()

			if tc.wantFailure {
				require.False(t, success)
				require.Empty(t, requests, "nothing may reach the target when resolution fails")
				require.Contains(t, agentLogger.String(), tc.wantLogContains)

				return
			}

			require.True(t, success)
			require.Len(t, requests, 1)

			// Without this, an empty log would pass the leak check below.
			if tc.wantLogContains != "" {
				require.Contains(t, agentLogger.String(), tc.wantLogContains)
			}

			// Secret values must never reach the logs.
			require.NotContains(t, agentLogger.String(), wireTokenValue)
			require.NotContains(t, agentLogger.String(), wirePasswordValue)

			// Exact equality, never Contains: "Bearer ${secrets.api-token}" and
			// "Bearer resolved-token-value" both contain "Bearer".
			require.Equal(t, tc.wantAuthorization, requests[0].header.Get("Authorization"))

			for header, value := range tc.wantHeader {
				require.Equal(t, value, requests[0].header.Get(header))
			}
		})
	}
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
			wantError: secrets.ErrSecretNotFound.Error(),
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

			cfg, err := buildPrometheusHTTPClientConfig(
				ctx,
				&settings,
				zerolog.New(&logs),
				secretStore,
				model.GlobalID(1),
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
