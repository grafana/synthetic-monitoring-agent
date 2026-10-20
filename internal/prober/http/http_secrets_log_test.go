package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	kitlog "github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type tenantStub struct {
	tenant *sm.Tenant
	err    error
}

func (s tenantStub) GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error) {
	return s.tenant, s.err
}

// TestSecretFailureReachesTheCheckLogs asserts what the owner of a check can
// read when one of its secret references does not resolve.
func TestSecretFailureReachesTheCheckLogs(t *testing.T) {
	const (
		missingRef       = "${secrets.does-not-exist}"
		proxyCredentials = "proxy-user:proxy-password"
		storeToken       = "secret-store-token"
		brokenProxyURL   = "http://" + proxyCredentials + "@%zz"
		internalAddr     = "10.0.0.1:4031"

		tokenRef   = "${secrets.api-token}"
		tokenName  = "api-token"
		tokenValue = "resolved-token-value"

		passwordName  = "api-password"
		passwordRef   = "${secrets.api-password}"
		passwordValue = "resolved-password-value"
	)

	// Example of sensitive detail that must not reach the tenant.
	internalDetail := []string{"127.0.0.1", internalAddr, "/decrypt", "dial tcp", "rpc error", storeToken}

	secretStoreAt := func(url string) secrets.SecretProvider {
		return secrets.NewSecretProvider(tenantStub{
			tenant: &sm.Tenant{Id: 1, SecretStore: &sm.SecretStore{Url: url, Token: storeToken}},
		}, time.Minute, zerolog.Nop())
	}

	serve := func(handler http.HandlerFunc) secrets.SecretProvider {
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)

		return secretStoreAt(server.URL)
	}

	status := func(code int) secrets.SecretProvider {
		return serve(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		})
	}

	values := map[string]string{
		"/secrets/" + tokenName + "/decrypt":    tokenValue,
		"/secrets/" + passwordName + "/decrypt": passwordValue,
	}

	secretsStoreMock := serve(func(w http.ResponseWriter, r *http.Request) {
		value, found := values[r.URL.Path]
		if !found {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"plaintext":"`+value+`"}`)
	})

	unreachableSecretStore := func() secrets.SecretProvider {
		server := httptest.NewServer(http.NotFoundHandler())
		server.Close()

		return secretStoreAt(server.URL)
	}()

	testcases := map[string]struct {
		settings sm.HttpSettings
		// secretStore overrides `secretsStoreMock` for this case only.
		secretStore secrets.SecretProvider
		// wantCheckLog is asserted against the check owner log stream.
		wantCheckLog []string
		// wantCheckLogEmpty nothing may reach that check stream log at all.
		wantCheckLogEmpty bool
		// wantAgentLog is detail the operator get but never the check owner.
		wantAgentLog []string
		// wantAbsent may not be in either log stream (agent and check).
		wantAbsent  []string
		wantSuccess bool
	}{
		"unknown secret in bearer token": {
			settings: sm.HttpSettings{BearerToken: missingRef},
			wantCheckLog: []string{
				`msg="Could not resolve a secret referenced by this check"`,
				`field="bearer token"`,
				"secret=does-not-exist",
				`reason="the secret does not exist"`,
			},
		},

		"unknown secret in basic auth password": {
			settings: sm.HttpSettings{
				BasicAuth: &sm.BasicAuth{Username: "check-user", Password: missingRef},
			},
			wantCheckLog: []string{`field="basic auth password"`, "secret=does-not-exist"},
		},

		"unknown secret in CA cert": {
			settings: sm.HttpSettings{
				TlsConfig: &sm.TLSConfig{CACert: []byte(missingRef)},
			},
			wantCheckLog: []string{`field="CA cert"`, "secret=does-not-exist"},
		},

		"unknown secret in client key": {
			settings: sm.HttpSettings{
				TlsConfig: &sm.TLSConfig{ClientKey: []byte(missingRef)},
			},
			wantCheckLog: []string{`field="client key"`, "secret=does-not-exist"},
		},

		// Validation by the `interpolation.isValidSecretName`
		"invalid secret name": {
			settings:     sm.HttpSettings{BearerToken: "${secrets.My_Token}"},
			wantCheckLog: []string{`field="bearer token"`, "invalid secret name", "lowercase letters"},
			wantAbsent:   []string{"My_Token"},
		},

		"secret store unreachable": {
			settings:     sm.HttpSettings{BearerToken: tokenRef},
			secretStore:  unreachableSecretStore,
			wantCheckLog: []string{"secret=" + tokenName, `reason="the secret store is unavailable"`},
			wantAgentLog: []string{"/secrets/" + tokenName + "/decrypt"},
		},

		"secret store rejects the agent's token": {
			settings:     sm.HttpSettings{BearerToken: tokenRef},
			secretStore:  status(http.StatusUnauthorized),
			wantCheckLog: []string{`reason="the secret store rejected the agent's access token"`},
		},

		"secret store fails": {
			settings:     sm.HttpSettings{BearerToken: tokenRef},
			secretStore:  status(http.StatusServiceUnavailable),
			wantCheckLog: []string{`reason="the secret store is unavailable"`},
			wantAgentLog: []string{"status 503"},
		},

		"secret store sends a body that is not JSON": {
			settings: sm.HttpSettings{BearerToken: tokenRef},
			secretStore: serve(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, "{")
			}),
			wantCheckLog: []string{`reason="the secret store returned an unexpected response"`},
		},

		"SM API unavailable": {
			settings: sm.HttpSettings{BearerToken: tokenRef},
			secretStore: secrets.NewSecretProvider(tenantStub{
				err: errors.New(`rpc error: code = Unavailable desc = connection error: desc = "transport: Error while dialing: dial tcp ` + internalAddr + `: connect: connection refused"`),
			}, time.Minute, zerolog.Nop()),
			wantCheckLog: []string{`reason="the secret store is unavailable"`},
			wantAgentLog: []string{internalAddr},
		},

		"no secret store for the stack": {
			settings: sm.HttpSettings{BearerToken: tokenRef},
			secretStore: secrets.NewSecretProvider(tenantStub{
				tenant: &sm.Tenant{Id: 1},
			}, time.Minute, zerolog.Nop()),
			wantCheckLog: []string{`reason="no secret store is configured for this stack"`},
		},

		"a resolved bearer token is not in the logs when basic auth fails": {
			settings: sm.HttpSettings{
				BearerToken: tokenRef,
				BasicAuth:   &sm.BasicAuth{Username: "check-user", Password: missingRef},
			},
			wantCheckLog: []string{`field="basic auth password"`, "secret=does-not-exist"},
		},

		"a broken proxy URL stays out of the check logs": {
			settings:          sm.HttpSettings{ProxyURL: brokenProxyURL},
			wantCheckLogEmpty: true,
			wantAgentLog:      []string{"parsing proxy URL"},
		},

		"a broken OAuth2 proxy URL stays out of the check logs": {
			settings: sm.HttpSettings{
				Oauth2Config: &sm.OAuth2Config{
					ClientId: "client",
					TokenURL: "http://127.0.0.1/token",
					ProxyURL: brokenProxyURL,
				},
			},
			wantCheckLogEmpty: true,
			wantAgentLog:      []string{"parsing proxy URL"},
		},

		"a resolved password is not in the logs when the proxy URL is broken": {
			settings: sm.HttpSettings{
				BasicAuth: &sm.BasicAuth{Username: "check-user", Password: passwordRef},
				ProxyURL:  brokenProxyURL,
			},
			wantCheckLogEmpty: true,
		},

		"a reference that resolves reports nothing": {
			settings:    sm.HttpSettings{BearerToken: tokenRef},
			wantSuccess: true,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := testhelper.Context(context.Background(), t)
			t.Cleanup(cancel)

			target := newRequestRecorder(t)
			settings := tc.settings

			store := tc.secretStore
			if store == nil {
				store = secretsStoreMock
			}

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
					Settings:         sm.CheckSettings{Http: &settings},
				},
			}

			var (
				agentLog testhelper.LogBuffer
				checkLog bytes.Buffer
			)

			prober, err := NewProber(ctx, check, zerolog.New(&agentLog), http.Header{}, store)
			require.NoError(t, err)

			success, _ := prober.Probe(
				ctx,
				check.Target,
				prometheus.NewPedanticRegistry(),
				kitlog.NewLogfmtLogger(&checkLog),
				"test-execution-id",
			)

			require.Equal(t, tc.wantSuccess, success)

			if !tc.wantSuccess {
				require.Contains(t, agentLog.String(), "failed to build config for HTTP probe")
			}

			for _, want := range tc.wantAgentLog {
				require.Contains(t, agentLog.String(), want)
			}

			// Neither stream may have a resolved value or the proxy credentials.
			for _, log := range []string{checkLog.String(), agentLog.String()} {
				require.NotContains(t, log, tokenValue)
				require.NotContains(t, log, passwordValue)
				require.NotContains(t, log, proxyCredentials)

				for _, absent := range tc.wantAbsent {
					require.NotContains(t, log, absent)
				}
			}

			if tc.wantCheckLogEmpty {
				require.Empty(t, checkLog.String())

				return
			}

			// A success. The check log is never empty past this point.
			require.NotEmpty(t, checkLog.String())

			if tc.wantSuccess {
				require.NotContains(t, checkLog.String(), "Could not resolve a secret")

				return
			}

			for _, detail := range internalDetail {
				require.NotContains(t, checkLog.String(), detail)
			}

			for _, want := range tc.wantCheckLog {
				require.Contains(t, checkLog.String(), want)
			}
		})
	}
}
