package cals

import (
	"context"
	"fmt"
	"testing"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/require"
)

type mockTenantProvider struct {
	tenant *sm.Tenant
	err    error
}

func (m mockTenantProvider) GetTenant(_ context.Context, _ *sm.TenantInfo) (*sm.Tenant, error) {
	return m.tenant, m.err
}

func TestTenantCostAttributionLabels_GetCostAttributionLabels(t *testing.T) {
	testcases := map[string]struct {
		tenantProvider                *mockTenantProvider
		expectedCostAttributionLabels []string
		expectError                   bool
	}{
		"cals should match up": {
			tenantProvider: &mockTenantProvider{
				tenant: &sm.Tenant{
					CostAttributionLabels: []string{"this", "is", "a", "test"},
				},
			},
			expectedCostAttributionLabels: []string{"this", "is", "a", "test"},
			expectError:                   false,
		},
		"Handle returning an error": {
			tenantProvider: &mockTenantProvider{
				tenant: &sm.Tenant{},
				err:    fmt.Errorf("error getting tenant info"),
			},
			expectedCostAttributionLabels: []string{},
			expectError:                   true,
		},
	}
	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			tcal := NewCostAttributionLabels(testcase.tenantProvider)
			cals, err := tcal.CostAttributionLabels(context.Background(), 1)
			if testcase.expectError {
				require.ErrorIs(t, err, ErrTenantProvider)
			}
			require.ElementsMatch(t, cals, testcase.expectedCostAttributionLabels)
		})
	}
}
