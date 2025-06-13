package secrets

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type mockTenantProvider struct {
	tenant *sm.Tenant
	err    error
}

func (m *mockTenantProvider) GetTenant(ctx context.Context, info *sm.TenantInfo) (*sm.Tenant, error) {
	return m.tenant, m.err
}

func TestGetSecretCredentials_Success(t *testing.T) {
	mockSecretStore := &sm.SecretStore{}
	mockTenant := sm.Tenant{SecretStore: mockSecretStore}
	mockTenantProvider := &mockTenantProvider{tenant: &mockTenant}
	ts := NewTenantSecrets(mockTenantProvider, testhelper.Logger(t))
	ctx := context.Background()
	tenantID := model.GlobalID(1234)

	store, err := ts.GetSecretCredentials(ctx, tenantID)
	require.NoError(t, err)
	require.Equal(t, mockSecretStore, store)
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

func TestGetSecretCredentials_Error(t *testing.T) {
	getTenantErr := errors.New("tenant not found")
	mockTenantProvider := &mockTenantProvider{err: getTenantErr}
	ts := NewTenantSecrets(mockTenantProvider, testhelper.Logger(t))
	ctx := context.Background()
	tenantID := model.GlobalID(1234)

	_, err := ts.GetSecretCredentials(ctx, tenantID)
	require.Error(t, err)
	require.Equal(t, getTenantErr, err)
}

func TestTenantSecrets_GetSecretValue_NoSecretStore(t *testing.T) {
	logger := zerolog.New(io.Discard)

	// Mock tenant provider that returns a tenant without secret store
	tenantProvider := &mockTenantProvider{
		tenant: &sm.Tenant{
			Id:          123,
			SecretStore: nil,
		},
	}

	ts := NewTenantSecrets(tenantProvider, logger)

	_, err := ts.GetSecretValue(context.Background(), model.GlobalID(123), "test-secret")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no secret store configured")
}

func TestTenantSecrets_GetSecretValue_InvalidSecretStore(t *testing.T) {
	logger := zerolog.New(io.Discard)

	// Mock tenant provider that returns a tenant with invalid secret store URL
	tenantProvider := &mockTenantProvider{
		tenant: &sm.Tenant{
			Id: 123,
			SecretStore: &sm.SecretStore{
				Url:   "invalid-url",
				Token: "test-token",
			},
		},
	}

	ts := NewTenantSecrets(tenantProvider, logger)

	_, err := ts.GetSecretValue(context.Background(), model.GlobalID(123), "test-secret")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get secret from GSM")
}
