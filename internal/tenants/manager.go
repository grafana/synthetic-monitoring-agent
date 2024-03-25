package tenants

import (
	"context"
	"sync"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type Manager struct {
	tenantCh      <-chan sm.Tenant
	tenantsClient sm.TenantsClient
	timeout       time.Duration
	tenantsMutex  sync.Mutex
	tenants       map[int64]*tenantInfo
}

var _ pusher.TenantProvider = &Manager{}

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
func NewManager(ctx context.Context, tenantsClient sm.TenantsClient, tenantCh <-chan sm.Tenant, timeout time.Duration) *Manager {
	tm := &Manager{
		tenantCh:      tenantCh,
		tenantsClient: tenantsClient,
		timeout:       timeout,
		tenants:       make(map[int64]*tenantInfo),
	}

	go tm.run(ctx)

	return tm
}

func (tm *Manager) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case tenant := <-tm.tenantCh:
			tm.updateTenant(tenant)
		}
	}
}

func (tm *Manager) updateTenant(tenant sm.Tenant) {
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

// GetTenant retrieves the tenant specified by `req`, either from a local cache
// or by making a request to the API. Notice that this method will favour
// returning expired tenant data from the cache if new data can not be retrieved
// from the API.
func (tm *Manager) GetTenant(ctx context.Context, req *sm.TenantInfo) (*sm.Tenant, error) {
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

	// If there is a valid tenant in the cache, return it
	if info.validUntil.After(now) {
		return info.tenant, nil
	}

	// Request the tenant to the API
	tenant, err := tm.tenantsClient.GetTenant(ctx, req)
	// Treat every error in the same way, whether it's network or app related.
	// As example of application errors: If the API has issues reaching the DB,
	// we still don't want to block the agents. If the tenant is disabled, it
	// should be propagated through other paths, and this component should act
	// "silly" on it.
	if err != nil && (!found || info.tenant == nil) {
		// Only return error if tenant was not found in the cache or
		// is not a valid entry, and can not be retrieved from the API
		return nil, err
	}

	// If tenant was retrieved from the API, update it in the cache
	if err == nil {
		info.validUntil = time.Now().Add(tm.timeout)
		info.tenant = tenant
	}

	// At this point we are either returning the new tenant data retrieved
	// from the API, or the stale tenant data that was present in the cache
	return info.tenant, nil
}
