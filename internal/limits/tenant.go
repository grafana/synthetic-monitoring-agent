package limits

import (
	"context"
	"errors"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

const (
	// Tenant limits (defaults)

	maxMetricLabels = sm.MaxMetricLabels
	maxLogLabels    = sm.MaxLogLabels
)

var (
	ErrTooManyMetricLabels = errors.New("too many metric labels")
	ErrTooManyLogLabels    = errors.New("too many log labels")
)

type TenantProvider interface {
	GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error)
}

// TenantLimits provides functionalities to query and validate metrics and log
// labels limits for a particular tenant.
type TenantLimits struct {
	tp TenantProvider
}

func NewTenantLimits(tp TenantProvider) *TenantLimits {
	return &TenantLimits{
		tp,
	}
}

// MetricLabels returns the metric labels limit for the specified tenant.
func (tl *TenantLimits) MetricLabels(ctx context.Context, tenantID model.GlobalID) (int, error) {
	tenant, err := tl.tp.GetTenant(ctx, &sm.TenantInfo{
		Id: int64(tenantID),
	})
	if err != nil {
		return 0, err
	}

	max := maxMetricLabels
	if tenant.Limits != nil {
		max = int(tenant.Limits.MaxMetricLabels)
	}

	return max, nil
}

// LogLabels returns the log labels limit for the specified tenant.
func (tl *TenantLimits) LogLabels(ctx context.Context, tenantID model.GlobalID) (int, error) {
	tenant, err := tl.tp.GetTenant(ctx, &sm.TenantInfo{
		Id: int64(tenantID),
	})
	if err != nil {
		return 0, err
	}

	max := maxLogLabels
	if tenant.Limits != nil {
		max = int(tenant.Limits.MaxLogLabels)
	}

	return max, nil
}

// ValidateMetricLabels validates the given number of metric labels against the specific tenant limits.
func (tl *TenantLimits) ValidateMetricLabels(ctx context.Context, tenantID model.GlobalID, n int) error {
	max, err := tl.MetricLabels(ctx, tenantID)
	if err != nil {
		return err
	}

	if n > max {
		return ErrTooManyMetricLabels
	}

	return nil
}

// ValidateLogLabels validates the given number of log labels against the specific tenant limits.
func (tl *TenantLimits) ValidateLogLabels(ctx context.Context, tenantID model.GlobalID, n int) error {
	max, err := tl.LogLabels(ctx, tenantID)
	if err != nil {
		return err
	}

	if n > max {
		return ErrTooManyLogLabels
	}

	return nil
}
