package limits

import (
	"context"
	"errors"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/require"
)

var errTestTenantNotFound = errors.New("tenant not found")

type testTenantProvider struct {
	tenants map[int64]*sm.Tenant
}

func (tp *testTenantProvider) GetTenant(ctx context.Context, ti *sm.TenantInfo) (*sm.Tenant, error) {
	if t, ok := tp.tenants[ti.Id]; ok {
		return t, nil
	}
	return nil, errTestTenantNotFound
}

var tp = &testTenantProvider{
	tenants: map[int64]*sm.Tenant{
		// Tenant with specific limits that override defaults
		1: {
			Id: 1,
			Limits: &sm.TenantLimits{
				MaxMetricLabels: 30,
				MaxLogLabels:    25,
			},
		},
		// Tenant without specific limits
		2: {
			Id:     2,
			Limits: nil,
		},
	},
}

func TestMetricLabels(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		tenantID  model.GlobalID
		expLabels int
		expErr    error
	}{
		{
			name:      "specific tenant limits",
			tenantID:  1,
			expLabels: 30,
		},
		{
			name:      "default tenant limits",
			tenantID:  2,
			expLabels: maxMetricLabels,
		},
		{
			name:     "expect error",
			tenantID: 3,
			expErr:   errTestTenantNotFound,
		},
	}

	var (
		ctx = context.Background()
		tl  = NewTenantLimits(tp)
	)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ll, err := tl.MetricLabels(ctx, tc.tenantID)
			if err != nil {
				require.Equal(t, tc.expErr, err)
			}
			require.Equal(t, tc.expLabels, ll)
		})
	}
}

func TestLogLabels(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		tenantID  model.GlobalID
		expLabels int
		expErr    error
	}{
		{
			name:      "specific tenant limits",
			tenantID:  1,
			expLabels: 25,
		},
		{
			name:      "default tenant limits",
			tenantID:  2,
			expLabels: maxLogLabels,
		},
		{
			name:     "expect error",
			tenantID: 3,
			expErr:   errTestTenantNotFound,
		},
	}

	var (
		ctx = context.Background()
		tl  = NewTenantLimits(tp)
	)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ll, err := tl.LogLabels(ctx, tc.tenantID)
			if err != nil {
				require.Equal(t, tc.expErr, err)
			}
			require.Equal(t, tc.expLabels, ll)
		})
	}
}

func TestValidateMetricLabels(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		tenantID model.GlobalID
		nLabels  int
		expErr   error
	}{
		{
			name:     "below tenant metric labels limit",
			tenantID: 1,
			nLabels:  25,
		},
		{
			name:     "equal tenant metric labels limit",
			tenantID: 1,
			nLabels:  30,
		},
		{
			name:     "below default metric labels limit",
			tenantID: 2,
			nLabels:  15,
		},
		{
			name:     "over tenant metric labels limit",
			tenantID: 1,
			nLabels:  35,
			expErr:   ErrTooManyMetricLabels,
		},
		{
			name:     "over tenant metric labels limit",
			tenantID: 2,
			nLabels:  21,
			expErr:   ErrTooManyMetricLabels,
		},
	}

	var (
		ctx = context.Background()
		tl  = NewTenantLimits(tp)
	)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tl.ValidateMetricLabels(ctx, tc.tenantID, tc.nLabels)
			require.Equal(t, tc.expErr, err)
		})
	}
}

func TestValidateLogLabels(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		tenantID model.GlobalID
		nLabels  int
		expErr   error
	}{
		{
			name:     "below tenant log labels limit",
			tenantID: 1,
			nLabels:  21,
		},
		{
			name:     "equal tenant log labels limit",
			tenantID: 1,
			nLabels:  25,
		},
		{
			name:     "below default log labels limit",
			tenantID: 2,
			nLabels:  10,
		},
		{
			name:     "over tenant log labels limit",
			tenantID: 1,
			nLabels:  29,
			expErr:   ErrTooManyLogLabels,
		},
		{
			name:     "over tenant log labels limit",
			tenantID: 2,
			nLabels:  18,
			expErr:   ErrTooManyLogLabels,
		},
	}

	var (
		ctx = context.Background()
		tl  = NewTenantLimits(tp)
	)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tl.ValidateLogLabels(ctx, tc.tenantID, tc.nLabels)
			require.Equal(t, tc.expErr, err)
		})
	}
}
