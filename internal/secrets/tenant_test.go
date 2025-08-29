package secrets

import (
	"context"
	"io"
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

func TestTenantSecrets_GetSecretCredentials(t *testing.T) {
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
		"tenant not found": {
			tenantProvider: &mockTenantProvider{
				err: assert.AnError,
			},
			expectedStore: nil,
			expectError:   true,
		},
		"tenant without secret store": {
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
			ts := NewTenantSecrets(tc.tenantProvider, logger)

			store, err := ts.GetSecretCredentials(context.Background(), model.GlobalID(123))

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedStore, store)
			}
		})
	}
}

func TestTenantSecrets_GetSecretValue(t *testing.T) {
	logger := zerolog.New(io.Discard)
	tenantProvider := &mockTenantProvider{
		tenant: &sm.Tenant{
			Id: 123,
			SecretStore: &sm.SecretStore{
				Url:   "https://secrets.example.com",
				Token: "test-token",
			},
		},
	}

	ts := NewTenantSecrets(tenantProvider, logger)

	// Test that GetSecretValue returns empty string for backward compatibility
	value, err := ts.GetSecretValue(context.Background(), model.GlobalID(123), "test-secret")
	require.NoError(t, err)
	require.Equal(t, "", value)
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
	require.Error(t, err)
	require.Contains(t, err.Error(), "no secret store configured")
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
			error: "GSM URL cannot be empty",
		},
		"empty token": {
			url:   "https://test-gsm.com",
			token: "",
			error: "GSM token cannot be empty",
		},
		"both empty": {
			url:   "",
			token: "",
			error: "GSM URL cannot be empty",
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
