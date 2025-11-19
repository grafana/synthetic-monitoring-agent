package cals

import (
	"context"
	"errors"
	"fmt"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

var (
	ErrTenantProvider = errors.New("fetching tenant data")
)

type TenantProvider interface {
	GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error)
}

// CostAttributionLabels has a TenantProvider that pulls data about a specific tenant
type CostAttributionLabels struct {
	provider TenantProvider
}

// NewCostAttributionLabels is a helper method to create a NewCostAttributionLabels provider
func NewCostAttributionLabels(provider TenantProvider) *CostAttributionLabels {
	return &CostAttributionLabels{
		provider: provider,
	}
}

// CostAttributionLabels will call TenantProvider.GetTenant to search for a specific tenant and returns Tenant.CostAttributionLabels
func (tcal CostAttributionLabels) CostAttributionLabels(ctx context.Context, tenantID model.GlobalID) ([]string, error) {
	tenant, err := tcal.provider.GetTenant(ctx, &sm.TenantInfo{
		Id: int64(tenantID),
	})

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTenantProvider, err)
	}

	return tenant.CostAttributionLabels, nil
}
