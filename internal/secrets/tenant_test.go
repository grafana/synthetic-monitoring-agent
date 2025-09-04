package secrets

import (
	"context"
	"io"
	"testing"

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
