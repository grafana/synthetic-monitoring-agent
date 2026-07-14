package labelmode

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type fakeTenantProvider struct {
	tenant *sm.Tenant
	err    error
}

func (f fakeTenantProvider) GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error) {
	return f.tenant, f.err
}

// TestForTenantReturnsTenantMode verifies the tenant's configured label mode is
// returned when the lookup succeeds.
func TestForTenantReturnsTenantMode(t *testing.T) {
	lm := New(fakeTenantProvider{
		tenant: &sm.Tenant{LabelMode: sm.LabelMode_LABEL_MODE_DUAL_WRITE},
	})

	mode, err := lm.ForTenant(context.Background(), model.GlobalID(1))
	require.NoError(t, err)
	require.Equal(t, sm.LabelMode_LABEL_MODE_DUAL_WRITE, mode)
}

// TestForTenantFallsBackToUnprefixedOnError verifies that a lookup failure
// returns UNPREFIXED alongside the error, so callers can degrade gracefully.
func TestForTenantFallsBackToUnprefixedOnError(t *testing.T) {
	lm := New(fakeTenantProvider{err: errors.New("boom")})

	mode, err := lm.ForTenant(context.Background(), model.GlobalID(1))
	require.Error(t, err)
	require.Equal(t, sm.LabelMode_LABEL_MODE_UNPREFIXED, mode)
}
