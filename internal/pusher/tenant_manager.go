package pusher

import (
	"context"
	"sync"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type TenantManager struct {
	tenantCh      <-chan sm.Tenant
	tenantsClient sm.TenantsClient
	timeout       time.Duration
	tenantsMutex  sync.Mutex
	tenants       map[int64]*tenantInfo
}

var _ TenantProvider = &TenantManager{}

type tenantInfo struct {
	mutex      sync.Mutex // protects the entire structure
	validUntil time.Time
	tenant     *sm.Tenant
}

// NewTenantManager creates a new tenant manager that is able to
// retrieve tenants from the remote API using the specified
// tenantsClient or receive them over the provided tenantCh channel. It
// will keep them for a duration no longer than `timeout`.
//
// A new goroutine is started which stops when the provided context is
// cancelled.
func NewTenantManager(ctx context.Context, tenantsClient sm.TenantsClient, tenantCh <-chan sm.Tenant, timeout time.Duration) *TenantManager {
	tm := &TenantManager{
		tenantCh:      tenantCh,
		tenantsClient: tenantsClient,
		timeout:       timeout,
		tenants:       make(map[int64]*tenantInfo),
	}

	go tm.run(ctx)

	return tm
}

func (tm *TenantManager) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case tenant := <-tm.tenantCh:
			tm.updateTenant(tenant)
		}
	}
}

func (tm *TenantManager) updateTenant(tenant sm.Tenant) {
	tm.tenantsMutex.Lock()

	info, found := tm.tenants[tenant.Id]
	if !found {
		info = new(tenantInfo)
		tm.tenants[tenant.Id] = info
	}

	tm.tenantsMutex.Unlock()

	// There's a window here where GetTenant got the lock before
	// this function, didn't find the tenant, created a new one and
	// added it. In that case `found` above would be true and this
	// function would not create a new one. Now we are racing to
	// acquire info.mutex.
	//
	// 1. updateTenant acquires it first: it simply inserts the new
	// tenant and GetTenant sees it and uses it;
	//
	// 2. GetTenant acquires it first: That's why we need to release
	// the lock _before_ operating on info, so that we don't end up
	// waiting for GetTenant to fetch the information from the API
	// and blocking other goroutines from retrieving a different
	// tenant. In that case, when updateTenant acquires the lock
	// below, make sure the tenant we have is in fact newer than the
	// one that is already there, if there's one.

	info.mutex.Lock()
	if info.tenant == nil || info.tenant.Modified < tenant.Modified {
		info.validUntil = time.Now().Add(tm.timeout)
		info.tenant = &tenant
	}
	info.mutex.Unlock()
}

// GetTenant retrieves the tenant specified by `req`, either from a
// local cache or by making a request to the API.
func (tm *TenantManager) GetTenant(ctx context.Context, req *sm.TenantInfo) (*sm.Tenant, error) {
	tm.tenantsMutex.Lock()
	now := time.Now()
	info, found := tm.tenants[req.Id]
	if !found {
		info = new(tenantInfo)
		tm.tenants[req.Id] = info
	}
	tm.tenantsMutex.Unlock()

	info.mutex.Lock()
	defer info.mutex.Unlock()

	if info.validUntil.After(now) {
		return info.tenant, nil
	}

	tenant, err := tm.tenantsClient.GetTenant(ctx, req)

	if err == nil {
		info.validUntil = time.Now().Add(tm.timeout)
		info.tenant = tenant
	}

	return tenant, err
}
