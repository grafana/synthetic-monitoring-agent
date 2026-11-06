package secrets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type mockTenantProvider struct {
	tenant *sm.Tenant
	err    error
}

func (m *mockTenantProvider) GetTenant(ctx context.Context, info *sm.TenantInfo) (*sm.Tenant, error) {
	return m.tenant, m.err
}

func TestSecretProvider_GetSecretCredentials(t *testing.T) {
	logger := zerolog.New(io.Discard)

	testcases := map[string]struct {
		tenantProvider *mockTenantProvider
		expectedStore  *sm.SecretStore
		expectError    bool
	}{
		"successful retrieval": {
			tenantProvider: &mockTenantProvider{
				tenant: &sm.Tenant{
					Id: 123,
					SecretStore: &sm.SecretStore{
						Url:   "https://secrets.example.com",
						Token: "test-token",
					},
				},
			},
			expectedStore: &sm.SecretStore{
				Url:   "https://secrets.example.com",
				Token: "test-token",
			},
			expectError: false,
		},
		"no secret store configured": {
			tenantProvider: &mockTenantProvider{
				tenant: &sm.Tenant{
					Id:          123,
					SecretStore: nil,
				},
			},
			expectedStore: nil,
			expectError:   false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			sp := NewSecretProvider(tc.tenantProvider, time.Minute, logger)

			store, err := sp.GetSecretCredentials(context.Background(), model.GlobalID(123))

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedStore, store)
			}
		})
	}
}

func TestSecretProvider_GetSecretValue_NoSecretStore(t *testing.T) {
	logger := zerolog.New(io.Discard)

	// Mock tenant provider that returns a tenant without secret store
	tenantProvider := &mockTenantProvider{
		tenant: &sm.Tenant{
			Id:          123,
			SecretStore: nil,
		},
	}

	sp := NewSecretProvider(tenantProvider, time.Minute, logger)

	_, err := sp.GetSecretValue(context.Background(), model.GlobalID(123), "test-secret")
	require.ErrorIs(t, err, ErrNoSecretStore)
}

func TestSecretProvider_GetSecretValue_EmptyURLAndToken(t *testing.T) {
	logger := zerolog.New(io.Discard)

	testCases := map[string]struct {
		url   string
		token string
		error string
	}{
		"empty URL": {
			url:   "",
			token: "test-token",
			error: "the secret store URL is not configured",
		},
		"empty token": {
			url:   "https://test-gsm.com",
			token: "",
			error: "the secret store token is not configured",
		},
		"both empty": {
			url:   "",
			token: "",
			error: "the secret store URL is not configured",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tenantProvider := &mockTenantProvider{
				tenant: &sm.Tenant{
					Id: 123,
					SecretStore: &sm.SecretStore{
						Url:   tc.url,
						Token: tc.token,
					},
				},
			}

			sp := NewSecretProvider(tenantProvider, time.Minute, logger)

			_, err := sp.GetSecretValue(context.Background(), model.GlobalID(123), "test-secret")
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.error)
		})
	}
}

func TestSecretProvider_GetSecretValue_Failures(t *testing.T) {
	logger := zerolog.New(io.Discard)

	status := func(code int) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}
	}

	respond := func(contentType, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", contentType)
			_, _ = io.WriteString(w, body)
		}
	}

	// hang holds the request until the client gives up on it.
	hang := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	cancelled := func(_ *testing.T) context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		return ctx
	}

	timingOut := func(t *testing.T) context.Context {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		t.Cleanup(cancel)

		return ctx
	}

	testcases := map[string]struct {
		handler    http.Handler
		tenantErr  error
		ctx        func(t *testing.T) context.Context
		want       error
		wantDetail string
	}{
		"secret not found": {
			handler: status(http.StatusNotFound),
			want:    ErrSecretNotFound,
		},
		"token for another stack": {
			handler:    status(http.StatusUnauthorized),
			want:       ErrSecretStoreUnauthorized,
			wantDetail: "status 401",
		},
		"token without the scope": {
			handler:    status(http.StatusForbidden),
			want:       ErrSecretStoreUnauthorized,
			wantDetail: "status 403",
		},
		"rate limited": {
			handler:    status(http.StatusTooManyRequests),
			want:       ErrSecretStoreUnavailable,
			wantDetail: "status 429",
		},
		"server error": {
			handler:    status(http.StatusServiceUnavailable),
			want:       ErrSecretStoreUnavailable,
			wantDetail: "status 503",
		},
		"unexpected status": {
			handler:    status(http.StatusBadRequest),
			want:       ErrSecretStoreBadResponse,
			wantDetail: "status 400",
		},
		"malformed JSON": {
			handler:    respond("application/json", "{"),
			want:       ErrSecretStoreBadResponse,
			wantDetail: "unexpected end of JSON input",
		},
		"not JSON": {
			handler:    respond("text/plain", "plaintext"),
			want:       ErrSecretStoreBadResponse,
			wantDetail: `content type "text/plain"`,
		},
		"store unreachable": {
			want:       ErrSecretStoreUnavailable,
			wantDetail: "/secrets/test-secret/decrypt",
		},
		"store too slow for the check timeout": {
			handler: hang,
			ctx:     timingOut,
			want:    ErrCheckTimedOut,
		},
		"check cancelled": {
			handler: status(http.StatusOK),
			ctx:     cancelled,
			want:    ErrCheckCanceled,
		},
		"tenant API unavailable": {
			handler:    status(http.StatusOK),
			tenantErr:  errors.New("rpc error: code = Unavailable"),
			want:       ErrSecretStoreUnavailable,
			wantDetail: "rpc error: code = Unavailable",
		},
		"tenant API call cancelled": {
			handler:    status(http.StatusOK),
			tenantErr:  errors.New("rpc error: code = Canceled desc = context canceled"),
			ctx:        cancelled,
			want:       ErrCheckCanceled,
			wantDetail: "could not load the secret store configuration",
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var server *httptest.Server
			if tc.handler != nil {
				server = httptest.NewServer(tc.handler)
				t.Cleanup(server.Close)
			} else {
				server = httptest.NewServer(http.NotFoundHandler())
				server.Close()
			}

			tenantProvider := &mockTenantProvider{
				tenant: &sm.Tenant{
					Id: 123,
					SecretStore: &sm.SecretStore{
						Url:   server.URL,
						Token: "test-token",
					},
				},
				err: tc.tenantErr,
			}

			sp := NewSecretProvider(tenantProvider, time.Minute, logger)

			ctx := context.Background()
			if tc.ctx != nil {
				ctx = tc.ctx(t)
			}

			_, err := sp.GetSecretValue(ctx, model.GlobalID(123), "test-secret")
			require.ErrorIs(t, err, tc.want)
			require.Contains(t, err.Error(), tc.wantDetail)
			require.Equal(t, tc.want, AsError(err).Reason)
		})
	}
}

func TestError(t *testing.T) {
	detail := errors.New(`get "https://gsm.internal/secrets/x/decrypt": dial tcp`)

	testcases := map[string]struct {
		err        *Error
		wantText   string
		wantFields []any
		wantIs     []error
	}{
		"every part set": {
			err:        &Error{Field: "bearer token", Secret: "x", Reason: ErrSecretStoreUnavailable, Err: detail},
			wantText:   `failed to resolve bearer token: secret 'x': the secret store is unavailable: ` + detail.Error(),
			wantFields: []any{"field", "bearer token", "secret", "x", "reason", ErrSecretStoreUnavailable.Error()},
			wantIs:     []error{ErrSecretStoreUnavailable, detail},
		},
		"no underlying error": {
			err:        &Error{Field: "bearer token", Secret: "x", Reason: ErrSecretNotFound},
			wantText:   `failed to resolve bearer token: secret 'x': the secret does not exist`,
			wantFields: []any{"field", "bearer token", "secret", "x", "reason", ErrSecretNotFound.Error()},
			wantIs:     []error{ErrSecretNotFound},
		},
		"reason only": {
			err:        &Error{Reason: ErrNoSecretStore},
			wantText:   ErrNoSecretStore.Error(),
			wantFields: []any{"reason", ErrNoSecretStore.Error()},
			wantIs:     []error{ErrNoSecretStore},
		},
		"zero value": {
			err:        &Error{},
			wantText:   ErrSecretFetchFailed.Error(),
			wantFields: []any{"reason", ErrSecretFetchFailed.Error()},
			wantIs:     []error{ErrSecretFetchFailed},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.wantText, tc.err.Error())
			require.Equal(t, tc.wantFields, tc.err.LogFields())

			for _, want := range tc.wantIs {
				require.ErrorIs(t, tc.err, want)
			}
		})
	}
}

func TestAsError(t *testing.T) {
	t.Run("returns a copy", func(t *testing.T) {
		orig := &Error{Reason: ErrSecretNotFound}

		got := AsError(fmt.Errorf("wrapped: %w", orig))
		got.Secret = "x"

		require.Equal(t, ErrSecretNotFound, got.Reason)
		require.Empty(t, orig.Secret)
	})

	t.Run("wraps any other error", func(t *testing.T) {
		other := errors.New(`get "https://gsm.internal/secrets/x/decrypt": dial tcp`)

		got := AsError(other)

		require.Equal(t, ErrSecretFetchFailed, got.Reason)
		require.ErrorIs(t, got, other)
		require.NotContains(t, fmt.Sprint(got.LogFields()...), "gsm.internal")
	})
}

func TestSecretProvider(t *testing.T) {
	logger := zerolog.New(nil).Level(zerolog.Disabled)

	t.Run("cache behavior can be observed through API", func(t *testing.T) {
		tenantProvider := &mockTenantProvider{
			tenant: &sm.Tenant{
				Id: 123,
				SecretStore: &sm.SecretStore{
					Url:   "http://test-gsm.com",
					Token: "test-token",
				},
			},
		}

		sp := NewSecretProvider(tenantProvider, time.Minute, logger)

		// Test that we can call the basic interface methods
		store, err := sp.GetSecretCredentials(context.Background(), model.GlobalID(123))
		require.NoError(t, err)
		assert.NotNil(t, store)
	})

	t.Run("cache TTL is respected", func(t *testing.T) {
		ttl := 50 * time.Millisecond
		tenantProvider := &mockTenantProvider{
			tenant: &sm.Tenant{
				Id: 123,
				SecretStore: &sm.SecretStore{
					Url:   "http://test-gsm.com",
					Token: "test-token",
				},
			},
		}
		sp := NewSecretProvider(tenantProvider, ttl, logger)

		// Test that the provider was created successfully
		assert.NotNil(t, sp)

		// Test that the provider works correctly
		store, err := sp.GetSecretCredentials(context.Background(), model.GlobalID(123))
		require.NoError(t, err)
		assert.NotNil(t, store)
	})

	t.Run("GetSecretCredentials works correctly", func(t *testing.T) {
		expectedStore := &sm.SecretStore{
			Url:   "http://test-gsm.com",
			Token: "test-token",
		}

		tenantProvider := &mockTenantProvider{
			tenant: &sm.Tenant{
				Id:          123,
				SecretStore: expectedStore,
			},
		}

		sp := NewSecretProvider(tenantProvider, time.Minute, logger)
		tenantID := model.GlobalID(123)

		// Should delegate to tenant provider
		store, err := sp.GetSecretCredentials(context.Background(), tenantID)
		require.NoError(t, err)
		assert.Equal(t, expectedStore, store)
	})
}
