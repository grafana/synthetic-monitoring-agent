package tenants

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
)

// isSecretStoreConfigured returns true if the SecretStore has both URL and token configured.
func isSecretStoreConfigured(secretStore *sm.SecretStore) bool {
	return secretStore != nil && secretStore.Url != "" && secretStore.Token != ""
}

type Manager struct {
	tenantCh      <-chan sm.Tenant
	tenantsClient sm.TenantsClient
	timeout       time.Duration
	tenantsMutex  sync.Mutex
	tenants       map[int64]*tenantInfo
	logger        zerolog.Logger
}

var _ pusher.TenantProvider = &Manager{}

type tenantInfo struct {
	mutex      sync.Mutex // protects the entire structure
	validUntil time.Time
	tenant     *sm.Tenant
}

// NewManager creates a new tenant manager that is able to
// retrieve tenants from the remote API using the specified
// tenantsClient or receive them over the provided tenantCh channel. It
// will keep them for a duration no longer than `timeout`.
//
// A new goroutine is started which stops when the provided context is
// cancelled.
func NewManager(ctx context.Context, tenantsClient sm.TenantsClient, tenantCh <-chan sm.Tenant, timeout time.Duration, logger zerolog.Logger) *Manager {
	tm := &Manager{
		tenantCh:      tenantCh,
		tenantsClient: tenantsClient,
		timeout:       timeout,
		tenants:       make(map[int64]*tenantInfo),
		logger:        logger,
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

// calculateValidUntil determines the expiration time for a tenant based on the timeout
// and the secret store expiration date (if set), returning the earlier of the two.
// If the secret store is not properly configured, the cache is considered invalid immediately.
func (tm *Manager) calculateValidUntil(tenant *sm.Tenant) time.Time {
	now := time.Now()

	tm.logger.Debug().
		Int64("tenantId", tenant.Id).
		Bool("hasSecretStore", tenant.SecretStore != nil).
		Bool("secretStoreIsConfigured", isSecretStoreConfigured(tenant.SecretStore)).
		Float64("secretStoreExpiry", func() float64 {
			if tenant.SecretStore != nil {
				return tenant.SecretStore.Expiry
			}
			return 0
		}()).
		Dur("configuredTimeout", tm.timeout).
		Msg("calculating tenant validity period")

	// If secret store is not properly configured, consider cache invalid immediately
	if !isSecretStoreConfigured(tenant.SecretStore) {
		tm.logger.Warn().
			Int64("tenantId", tenant.Id).
			Msg("secret store not properly configured, considering cache invalid")
		return now
	}

	if tenant.SecretStore.Expiry > 0 {
		seconds, nanonseconds := math.Modf(tenant.SecretStore.Expiry)
		// Subtract MaxScriptedTimeout to ensure the token is valid for the maximum running time.
		expirationTime := time.Unix(int64(seconds), int64(nanonseconds*1e9))

		delta := expirationTime.Sub(now)

		tm.logger.Debug().
			Int64("tenantId", tenant.Id).
			Time("secretStoreExpirationTime", expirationTime).
			Dur("deltaFromNow", delta).
			Dur("maxScriptedTimeout", sm.MaxScriptedTimeout).
			Msg("secret store expiry calculation")

		switch {
		case delta < 0:
			// The token is already expired, return the current time
			tm.logger.Warn().
				Int64("tenantId", tenant.Id).
				Time("secretStoreExpirationTime", expirationTime).
				Msg("secret store token already expired")
			return now

		case delta < sm.MaxScriptedTimeout:
			// The token is valid for less than MaxScriptedTimeout, return the expiration time
			tm.logger.Debug().
				Int64("tenantId", tenant.Id).
				Time("validUntil", expirationTime).
				Msg("using secret store expiration time (less than MaxScriptedTimeout)")
			return expirationTime

		default:
			// Reduce delta by sm.MaxScriptedTimeout to ensure the token is valid for the maximum running time.
			delta -= sm.MaxScriptedTimeout

			// Pick the smallest value between the calculated delta and the configured timeout.
			delta = min(delta, tm.timeout)

			validUntil := now.Add(delta)
			tm.logger.Debug().
				Int64("tenantId", tenant.Id).
				Time("validUntil", validUntil).
				Dur("calculatedDelta", delta).
				Msg("using calculated validity period")
			return validUntil
		}
	}

	validUntil := now.Add(tm.timeout)
	tm.logger.Debug().
		Int64("tenantId", tenant.Id).
		Time("validUntil", validUntil).
		Msg("using configured timeout (no secret store expiry)")
	return validUntil
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
		if tenant.SecretStore == nil {
			tm.logger.Warn().
				Int64("tenantId", tenant.Id).
				Msg("tenant received from API without secret store details")
		} else if !isSecretStoreConfigured(tenant.SecretStore) {
			tm.logger.Warn().
				Int64("tenantId", tenant.Id).
				Msg("tenant received from API with incomplete secret store configuration")
		}

		// Set validUntil to the earlier of:
		// - Now + timeout
		// - Secret store expiration date (if set)
		info.validUntil = tm.calculateValidUntil(&tenant)
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
		tm.logger.Debug().
			Int64("tenantId", req.Id).
			Time("validUntil", info.validUntil).
			Dur("validFor", info.validUntil.Sub(now)).
			Msg("returning tenant from cache")

		return info.tenant, nil
	}

	// Request the tenant from the API
	tenant, err := tm.tenantsClient.GetTenant(ctx, req)
	// Treat every error in the same way, whether it's network or app related.
	// As example of application errors: If the API has issues reaching the DB,
	// we still don't want to block the agents. If the tenant is disabled, it
	// should be propagated through other paths, and this component should act
	// "silly" on it.
	if err != nil && (!found || info.tenant == nil) {
		tm.logger.Error().Err(err).Int64("tenantId", req.Id).Msg("failed to retrieve remote tenant information")
		// Only return error if tenant was not found in the cache or
		// is not a valid entry, and can not be retrieved from the API
		return nil, err
	}

	// If tenant was retrieved from the API, update it in the cache
	if err == nil {
		if tenant.SecretStore == nil {
			tm.logger.Warn().
				Int64("tenantId", req.Id).
				Msg("tenant retrieved from API without secret store details")
		} else if !isSecretStoreConfigured(tenant.SecretStore) {
			tm.logger.Warn().
				Int64("tenantId", req.Id).
				Msg("tenant retrieved from API with incomplete secret store configuration")
		}

		// Set validUntil to the earlier of:
		// - Now + timeout
		// - Secret store expiration date (if set)
		info.validUntil = tm.calculateValidUntil(tenant)
		info.tenant = tenant

		tm.logger.Debug().
			Int64("tenantId", req.Id).
			Time("validUntil", info.validUntil).
			Msg("tenant retrieved from API")
	}

	tm.logger.Debug().
		Int64("tenantId", req.Id).
		Time("validUntil", info.validUntil).
		Dur("validFor", info.validUntil.Sub(now)).
		Msg("returning tenant")

	// At this point we are either returning the new tenant data retrieved
	// from the API, or the stale tenant data that was present in the cache
	return info.tenant, nil
}
