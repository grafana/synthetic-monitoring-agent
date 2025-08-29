package secrets

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type SecretProvider interface {
	GetSecretCredentials(ctx context.Context, tenantID model.GlobalID) (*sm.SecretStore, error)
	GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error)
}

type TenantProvider interface {
	GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error)
}

// TenantSecrets provides backward compatibility with existing code
type TenantSecrets struct {
	tp     TenantProvider
	logger zerolog.Logger
}

// NewTenantSecrets creates a new TenantSecrets instance for backward compatibility
func NewTenantSecrets(tp TenantProvider, logger zerolog.Logger) *TenantSecrets {
	return &TenantSecrets{
		tp:     tp,
		logger: logger,
	}
}

// GetSecretCredentials gets the secret store configuration for a tenant (backward compatibility)
func (ts *TenantSecrets) GetSecretCredentials(ctx context.Context, tenantID model.GlobalID) (*sm.SecretStore, error) {
	if ts.logger.GetLevel() <= zerolog.DebugLevel {
		tenantID, regionID := model.GetLocalAndRegionIDs(tenantID)
		ts.logger.Debug().Int("regionID", regionID).Int64("tenantId", tenantID).Msg("getting secret credentials")
	}

	tenant, err := ts.tp.GetTenant(ctx, &sm.TenantInfo{
		Id: int64(tenantID),
	})
	if err != nil {
		ts.logger.Warn().Err(err).Int64("tenantId", int64(tenantID)).Msg("failed to get tenant")
		return nil, err
	}

	return tenant.SecretStore, nil
}

// GetSecretValue implements SecretProvider interface (backward compatibility)
func (ts *TenantSecrets) GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
	// For backward compatibility, return empty string
	// This will be replaced by the full implementation in PR 2
	return "", nil
}
