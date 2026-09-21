package http

import (
	"context"
	"encoding/base64"
	"errors"
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

// The secret names and values these tests share. The values are deliberately
// distinct from the references, so that a passthrough and a resolution can never
// be mistaken for each other.
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

// requestRecorder is a probe target that keeps the requests it receives, so that
// a test can assert on the exact bytes the prober put on the wire.
//
// The testserver package cannot be used for this. It validates the request
// method and echoes response headers, and it drops the inbound ones, which are
// the only thing of interest here.
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

// TestInterpolationOnTheWire asserts the exact credential bytes the agent sends
// to the target, for each shape of credential value. Resolution is
// unconditional, so the value is the only variable.
//
// The existing tests in this package cover resolveSecretValue and
// buildPrometheusHTTPClientConfig in isolation. This one goes through NewProber
// and Probe and reads the request off a recording target, so it also covers the
// wiring in between: which settings fields feed the resolver, how the resolved
// value reaches the Prometheus HTTP client, and what happens to the request when
// resolution fails.
//
// The nested and dollar-only cases are worth keeping. SecretRegex is unanchored,
// so a stray "${" earlier in a value does not prevent a well-formed reference
// later in it, and a "$" that does not open a placeholder is never special.
// Both properties are easy to break while every well-formed input still
// resolves.
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
		wantAuthorization string
		wantHeader        map[string]string
		wantFailure       bool
		wantLogContains   string
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
		},

		// As above, for the other credential field.

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

		// An unterminated "${" before a well-formed reference does not prevent
		// the reference from resolving, and is carried through as literal text.
		"secret reference after an unterminated placeholder": {
			settings: sm.HttpSettings{
				BearerToken: "prefix ${ " + wireTokenRef + " suffix",
			},
			wantAuthorization: "Bearer prefix ${ " + wireTokenValue + " suffix",
		},

		// The same when the reference sits inside the unterminated placeholder
		// rather than after it.
		"secret reference nested in an unterminated placeholder": {
			settings: sm.HttpSettings{
				BearerToken: "${" + wireTokenRef,
			},
			wantAuthorization: "Bearer ${" + wireTokenValue,
		},

		// The resolver only ever expands ${secrets.name}, so the literal
		// "secrets." prefix is the only thing that is meaningful here and a
		// bare ${name} is left alone.
		"variable syntax is not interpolated": {
			settings: sm.HttpSettings{
				BearerToken: "${api-token}",
			},
			wantAuthorization: "Bearer ${api-token}",
		},

		// A "$" that does not open a "${...}" placeholder is never special, and
		// "$$" has no meaning either, so a check configuration may hold a
		// literal dollar anywhere.
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

		// Custom headers are not one of the fields the agent resolves, so a
		// reference in a header value reaches the target as written.
		"secret reference in a custom header": {
			settings: sm.HttpSettings{
				Headers: []string{"X-Api-Key: " + wireTokenRef},
			},
			wantHeader: map[string]string{"X-Api-Key": wireTokenRef},
		},

		// A name that matches SecretRegex but fails isValidSecretName is a hard
		// error rather than a skip.
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

			// A resolution failure is reported through the logger handed to
			// NewProber, not through the check's own log stream, so that is the
			// one the failure cases have to capture. Wiring the buffer into
			// Probe's logger instead makes the assertion silently never fire.
			var logs testhelper.LogBuffer

			prober, err := NewProber(ctx, check, zerolog.New(&logs), http.Header{}, store)
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
				require.Contains(t, logs.String(), tc.wantLogContains)

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
		})
	}
}

// The TLS test fixtures. Byte-identity is what matters, so these do not need to
// be parseable certificates.
const (
	wireCACertPEM     = "-----BEGIN CERTIFICATE-----\nCA_CERT_CONTENT\n-----END CERTIFICATE-----\n"
	wireClientCertPEM = "-----BEGIN CERTIFICATE-----\nCLIENT_CERT_CONTENT\n-----END CERTIFICATE-----\n"
	wireClientKeyPEM  = "-----BEGIN PRIVATE KEY-----\nCLIENT_KEY_CONTENT\n-----END PRIVATE KEY-----\n"
)

// TestInterpolationInTLSMaterial covers the three TLS fields, which go through
// the same resolver as the two credential fields.
//
// It stops one level short of the wire on purpose. tls.SMtoProm writes each
// resolved field to a temporary file and hands blackbox exporter the path, so
// observing the material on the wire would need an unstarted TLS server, client
// certificate verification and a read of r.TLS.PeerCertificates. Reading the
// file back pins the same bytes in three lines. It complements
// TestBuildTLSConfig_WithSecrets, which asserts that a path was produced but not
// what landed in it.
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

		// A PEM blob holds no placeholder, so it is written out byte for byte.
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

			cfg, err := buildPrometheusHTTPClientConfig(
				ctx,
				&settings,
				testhelper.NewTestLogger(),
				secretStore,
				model.GlobalID(1),
			)

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
