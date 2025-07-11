package secrets

import (
	"context"
	"errors"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/assert"
)

type tenantProvider struct {
	tenant sm.Tenant
	err    error
}

func (m *tenantProvider) GetTenant(ctx context.Context, info *sm.TenantInfo) (*sm.Tenant, error) {
	return &m.tenant, m.err
}

func TestGetSecretCredentials_Success(t *testing.T) {
	mockSecretStore := &sm.SecretStore{}
	mockTenant := sm.Tenant{SecretStore: mockSecretStore}
	mockTenantProvider := &tenantProvider{tenant: mockTenant}
	ts := NewTenantSecrets(mockTenantProvider, testhelper.Logger(t))
	ctx := context.Background()
	tenantID := model.GlobalID(1234)

	secretStore, err := ts.GetSecretCredentials(ctx, tenantID)

	assert.NoError(t, err)
	assert.Equal(t, mockSecretStore, secretStore)
}

func TestGetSecretCredentials_Error(t *testing.T) {
	getTenantErr := errors.New("tenant not found")
	mockTenantProvider := &tenantProvider{err: getTenantErr}
	ts := NewTenantSecrets(mockTenantProvider, testhelper.Logger(t))
	ctx := context.Background()
	tenantID := model.GlobalID(1234)

	secretStore, err := ts.GetSecretCredentials(ctx, tenantID)

	assert.Error(t, err)
	assert.Nil(t, secretStore)
	assert.Equal(t, getTenantErr, err)
}
