package tenants

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type testTenantsClient struct {
	tenants      map[int64]sm.Tenant
	requestCount map[int64]int
}

var errTenantNotFound = errors.New("tenant not found")

func (c *testTenantsClient) GetTenant(ctx context.Context, in *sm.TenantInfo, opts ...grpc.CallOption) (*sm.Tenant, error) {
	c.requestCount[in.Id]++

	tenant, found := c.tenants[in.Id]
	if !found {
		return nil, errTenantNotFound
	}

	return &tenant, nil
}

func makeTenant(idx int) sm.Tenant {
	return sm.Tenant{
		Id:    int64(idx),
		OrgId: int64(idx * 1000),
		MetricsRemote: &sm.RemoteInfo{
			Name:     fmt.Sprintf("test-%d", idx),
			Url:      fmt.Sprintf("http://127.0.0.1/%d", idx),
			Username: fmt.Sprintf("user-%d", idx),
			Password: fmt.Sprintf("pw-%d", idx),
		},
	}
}

func TestTenantManagerGetTenant(t *testing.T) {
	tc := testTenantsClient{
		tenants: map[int64]sm.Tenant{
			1: makeTenant(1),
		},
		requestCount: make(map[int64]int),
	}

	deadline, hasTimeout := t.Deadline()
	if !hasTimeout {
		deadline = time.Now().Add(5 * time.Second)
	}

	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	tenantCh := make(chan sm.Tenant)

	tm := NewManager(ctx, &tc, tenantCh, 500*time.Millisecond)

	t1 := tc.tenants[1]

	// requesting an existent tenant should return it
	tenant, err := tm.GetTenant(ctx, &sm.TenantInfo{Id: t1.Id})
	require.NoError(t, err)
	require.NotNil(t, tenant)
	require.Equal(t, 1, tc.requestCount[t1.Id])
	require.Equal(t, t1, *tenant)

	// requesting the same tenant within a short period of time
	// should not cause a new request to the server, so the count
	// should remain at 1.
	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t1.Id})
	require.NoError(t, err)
	require.NotNil(t, tenant)
	require.Equal(t, 1, tc.requestCount[t1.Id])
	require.Equal(t, t1, *tenant)

	// requesting the same tenant after a longer time should evict
	// the existing tenant and make a new request; make sure we are
	// not getting a cached copy.
	time.Sleep(500 * time.Millisecond)

	t1.MetricsRemote.Password += "-new"

	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t1.Id})
	require.NoError(t, err)
	require.NotNil(t, tenant)
	require.Equal(t, 2, tc.requestCount[t1.Id])
	require.Equal(t, t1, *tenant)

	// create a new tenant but don't insert it in the cache
	t2 := makeTenant(2)

	// requesting a non-existent tenant should return an error
	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t2.Id})
	require.Error(t, err)
	require.Equal(t, errTenantNotFound, err)
	require.Nil(t, tenant)
	require.Equal(t, 1, tc.requestCount[t2.Id])

	// negative responses should not be cached
	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t2.Id})
	require.Error(t, err)
	require.Equal(t, errTenantNotFound, err)
	require.Nil(t, tenant)
	require.Equal(t, 2, tc.requestCount[t2.Id])

	// after adding the tenant, a new request for that tenant should
	// return the correct information
	tc.tenants[2] = t2
	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t2.Id})
	require.NoError(t, err)
	require.NotNil(t, tenant)
	require.Equal(t, 3, tc.requestCount[t2.Id])
	require.Equal(t, t2, *tenant)

	// create a new tenant and send it over the channel to the
	// tenant manager
	t3 := makeTenant(3)

	tenantCh <- t3
	// here we don't know if the tenant has been added to the list
	// of known tenants or not; busy-loop waiting for the tenant to
	// show up in the internal list kept by the tenant manager
	for i := 0; i < 100; i++ {
		tm.tenantsMutex.Lock()
		_, found := tm.tenants[t3.Id]
		tm.tenantsMutex.Unlock()
		if found {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t3.Id})
	require.NoError(t, err)
	require.NotNil(t, tenant)
	require.Equal(t, 0, tc.requestCount[t3.Id])
	require.Equal(t, t3, *tenant)
}
