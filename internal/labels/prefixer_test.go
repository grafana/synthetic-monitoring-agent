package labels

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

func TestGetPrefix(t *testing.T) {
	testcases := map[string]struct {
		tenantProvider *mockTenantProvider
		expectedPrefix string
		expectError    bool
	}{
		"default prefix should be used": {
			tenantProvider: &mockTenantProvider{
				tenant: &sm.Tenant{
					OmitLabelPrefix: false,
				},
			},
			expectedPrefix: "label_",
			expectError:    false,
		},
		"prefix should be omitted": {
			tenantProvider: &mockTenantProvider{
				tenant: &sm.Tenant{
					OmitLabelPrefix: true,
				},
			},
			expectedPrefix: "",
			expectError:    false,
		},
		"Handle returning an error": {
			tenantProvider: &mockTenantProvider{
				tenant: &sm.Tenant{},
				err:    fmt.Errorf("error getting tenant info"),
			},
			expectedPrefix: "label_",
			expectError:    true,
		},
	}
	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			prefixer := NewPrefixer(testcase.tenantProvider)
			prefix, err := prefixer.GetPrefix(context.Background(), 1)

			if testcase.expectError {
				require.ErrorIs(t, err, ErrTenantProvider)
			}
			require.Equal(t, prefix, testcase.expectedPrefix)
		})
	}
}
