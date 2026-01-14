package labels

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

// The legacy behavior is to always use the prefix "label_".
const labelPrefix = "label_"

type TenantProvider interface {
	GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error)
}

// Prefixer evalutes the prefix to use for tenant-defined labels on the `sm_check_info` metric.
//
// It supports migrating tenants from the legacy behavior to the new behavior by respecting the `OmitLabelPrefix` field in the tenant data.
type Prefixer struct {
	provider TenantProvider
}

// NewPrefixer is a helper method to create a new Prefixer instance.
func NewPrefixer(provider TenantProvider) *Prefixer {
	return &Prefixer{
		provider: provider,
	}
}

// GetPrefix will call TenantProvider.GetTenant to fetch the tenant data and evaluate the prefix to use for labels.
//
// If the tenant data cannot be fetched, the default prefix is returned.
func (prefixer *Prefixer) GetPrefix(ctx context.Context, tenantID model.GlobalID) (string, error) {
	tenant, err := prefixer.provider.GetTenant(ctx, &sm.TenantInfo{
		Id: int64(tenantID),
	})

	if err != nil {
		return labelPrefix, fmt.Errorf("%w: %v", ErrTenantProvider, err)
	}

	if tenant.OmitLabelPrefix {
		return "", nil
	}

	return labelPrefix, nil
}
