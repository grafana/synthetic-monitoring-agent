package secrets

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type SecretProvider interface {
	GetSecretCredentials(ctx context.Context, tenantID model.GlobalID) (*sm.SecretStore, error)
}

type TenantProvider interface {
	GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error)
}

type TenantSecrets struct {
	tp     TenantProvider
	logger zerolog.Logger
}

func NewTenantSecrets(tp TenantProvider, logger zerolog.Logger) *TenantSecrets {
	return &TenantSecrets{
		tp:     tp,
		logger: logger,
	}
}

func (ts *TenantSecrets) GetSecretCredentials(ctx context.Context, tenantID model.GlobalID) (*sm.SecretStore, error) {
	tenant, err := ts.tp.GetTenant(ctx, &sm.TenantInfo{
		Id: int64(tenantID),
	})
	if err != nil {
		return nil, err
	}
	return tenant.SecretStore, nil
}
