package labelmode

import (
	"context"
	"fmt"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// TenantProvider is the subset of the tenant manager interface required to
// look up a tenant's current label mode.
type TenantProvider interface {
	GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error)
}

// LabelMode provides the tenant's current label migration mode
// (PREFIXED / DUAL_WRITE / UNPREFIXED) by reading it from the GetTenant
// response, which is cached with a 15-minute TTL by the tenant manager.
type LabelMode struct {
	provider TenantProvider
}

// New creates a LabelMode provider backed by the given TenantProvider.
func New(provider TenantProvider) *LabelMode {
	return &LabelMode{provider: provider}
}

// ForTenant returns the label mode for the given tenant, defaulting to
// LABEL_MODE_PREFIXED if the tenant cannot be fetched.
func (lm *LabelMode) ForTenant(ctx context.Context, tenantID model.GlobalID) (sm.LabelMode, error) {
	tenant, err := lm.provider.GetTenant(ctx, &sm.TenantInfo{Id: int64(tenantID)})
	if err != nil {
		return sm.LabelMode_LABEL_MODE_PREFIXED, fmt.Errorf("fetching tenant label mode: %w", err)
	}
	return tenant.LabelMode, nil
}
