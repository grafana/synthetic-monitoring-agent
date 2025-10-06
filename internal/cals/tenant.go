package cals

import (
	"context"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type TenantProvider interface {
	GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error)
}

type TenantCals struct {
	provider TenantProvider
}

func NewTenantCals(provider TenantProvider) *TenantCals {
	return &TenantCals{
		provider: provider,
	}
}

func (tcal TenantCals) CostAttributionLabels(ctx context.Context, tenantID model.GlobalID) ([]string, error) {
	tenant, err := tcal.provider.GetTenant(ctx, &sm.TenantInfo{
		Id: int64(tenantID),
	})

	if err != nil {
		return nil, err
	}

	return tenant.CostAttributionLabels, nil
}
