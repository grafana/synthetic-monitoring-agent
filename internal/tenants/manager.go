package tenants

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/cache"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
)

const (
	// DefaultCacheTimeout is the default timeout for the tenant cache.
	DefaultCacheTimeout = 15 * time.Minute
	defaultCacheJitter  = 0.25 // 25% jitter
)

// isSecretStoreConfigured returns true if the SecretStore has both URL and token configured.
func isSecretStoreConfigured(secretStore *sm.SecretStore) bool {
	return secretStore != nil && secretStore.Url != "" && secretStore.Token != ""
}

type Manager struct {
	tenantCh      <-chan sm.Tenant
	tenantsClient sm.TenantsClient
	timeout       time.Duration
	cache         cache.Cache
	fetchMutexes  *xsync.Map[int64, *sync.Mutex] // for fetch deduplication.
	logger        zerolog.Logger
}

var _ pusher.TenantProvider = &Manager{}

// cachedTenant wraps a tenant with caching metadata
type cachedTenant struct {
	Tenant     *sm.Tenant
	ValidUntil time.Time
	Modified   float64 // Track Modified field to detect stale updates
}

// cacheKey generates a cache key for a tenant ID
func cacheKey(tenantID int64) string {
	return fmt.Sprintf("tenant:%d", tenantID)
}

// getFetchMutex returns a mutex for the given tenant ID to prevent concurrent API fetches
func (tm *Manager) getFetchMutex(tenantID int64) *sync.Mutex {
	actual, _ := tm.fetchMutexes.LoadOrStore(tenantID, &sync.Mutex{})

	return actual
}

// NewManager creates a new tenant manager that is able to
// retrieve tenants from the remote API using the specified
// tenantsClient or receive them over the provided tenantCh channel. It
// will keep them for a duration no longer than `timeout`.
//
// A new goroutine is started which stops when the provided context is
// cancelled.
func NewManager(ctx context.Context, tenantsClient sm.TenantsClient, tenantCh <-chan sm.Tenant, timeout time.Duration, cache cache.Cache, logger zerolog.Logger) *Manager {
	tm := &Manager{
		tenantCh:      tenantCh,
		tenantsClient: tenantsClient,
		timeout:       timeout,
		cache:         cache,
		logger:        logger,
		fetchMutexes:  xsync.NewMap[int64, *sync.Mutex](),
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

	// If secret store is configured but not properly set up, consider cache invalid immediately
	if tenant.SecretStore != nil && !isSecretStoreConfigured(tenant.SecretStore) {
		tm.logger.Warn().
			Int64("tenantId", tenant.Id).
			Msg("secret store not properly configured, considering cache invalid")
		return now
	}

	if tenant.SecretStore != nil && tenant.SecretStore.Expiry > 0 {
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

	// In case where the expiration time is set based on the default timeout,
	// apply a jitter to avoid many agents from hitting the API at the same time.
	jitterDuration := time.Duration(rand.Float64() * defaultCacheJitter * float64(tm.timeout))
	validUntil := now.Add(tm.timeout).Add(jitterDuration)

	tm.logger.Debug().
		Int64("tenantId", tenant.Id).
		Time("validUntil", validUntil).
		Msg("using configured timeout (no secret store expiry)")

	return validUntil
}

func (tm *Manager) updateTenant(tenant sm.Tenant) {
	// Use per-tenant mutex to serialize updates for the same tenant ID
	mutex := tm.getFetchMutex(tenant.Id)
	mutex.Lock()
	defer mutex.Unlock()

	ctx := context.Background()
	key := cacheKey(tenant.Id)

	// Check if there's already a cached tenant
	var existing cachedTenant
	err := tm.cache.Get(ctx, key, &existing)

	// Only update if this tenant is newer than what's in the cache
	if err == nil && existing.Tenant != nil && existing.Tenant.Modified >= tenant.Modified {
		tm.logger.Debug().
			Int64("tenantId", tenant.Id).
			Float64("existingModified", existing.Tenant.Modified).
			Float64("newModified", tenant.Modified).
			Msg("skipping tenant update, existing version is newer or equal")
		return
	}

	// Validate secret store configuration
	if tenant.SecretStore == nil {
		tm.logger.Warn().
			Int64("tenantId", tenant.Id).
			Msg("tenant received from API without secret store details")
	} else if !isSecretStoreConfigured(tenant.SecretStore) {
		tm.logger.Warn().
			Int64("tenantId", tenant.Id).
			Msg("tenant received from API with incomplete secret store configuration")
	}

	// Calculate expiration time using existing logic
	validUntil := tm.calculateValidUntil(&tenant)

	// Create cached tenant
	cached := cachedTenant{
		Tenant:     &tenant,
		ValidUntil: validUntil,
		Modified:   tenant.Modified,
	}

	// Store in cache without TTL - we'll check ValidUntil manually
	// This allows us to return stale data if the API is unavailable
	if err := tm.cache.Set(ctx, key, cached, 0); err != nil {
		tm.logger.Error().
			Err(err).
			Int64("tenantId", tenant.Id).
			Msg("failed to update tenant in cache")
		return
	}

	tm.logger.Debug().
		Int64("tenantId", tenant.Id).
		Time("validUntil", validUntil).
		Msg("tenant updated in cache")
}

// GetTenant retrieves the tenant specified by `req`, either from the cache
// or by making a request to the API. Notice that this method will favour
// returning expired tenant data from the cache if new data can not be retrieved
// from the API.
func (tm *Manager) GetTenant(ctx context.Context, req *sm.TenantInfo) (*sm.Tenant, error) {
	key := cacheKey(req.Id)
	now := time.Now()

	// Try to get tenant from cache
	var cached cachedTenant
	err := tm.cache.Get(ctx, key, &cached)

	// If found and still valid, return it
	if err == nil && cached.Tenant != nil && cached.ValidUntil.After(now) {
		tm.logger.Debug().
			Int64("tenantId", req.Id).
			Time("validUntil", cached.ValidUntil).
			Dur("validFor", cached.ValidUntil.Sub(now)).
			Msg("returning tenant from cache")
		return cached.Tenant, nil
	}

	// Cache miss or expired - need to fetch from API
	// Use per-tenant mutex to prevent concurrent fetches for the same tenant
	mutex := tm.getFetchMutex(req.Id)
	mutex.Lock()
	defer mutex.Unlock()

	// Check cache again after acquiring lock (another goroutine might have fetched it)
	err = tm.cache.Get(ctx, key, &cached)
	if err == nil && cached.Tenant != nil && cached.ValidUntil.After(now) {
		tm.logger.Debug().
			Int64("tenantId", req.Id).
			Msg("tenant was fetched by another goroutine")
		return cached.Tenant, nil
	}

	// Fetch from API
	tenant, fetchErr := tm.tenantsClient.GetTenant(ctx, req)

	// Treat every error in the same way, whether it's network or app related.
	// As example of application errors: If the API has issues reaching the DB,
	// we still don't want to block the agents. If the tenant is disabled, it
	// should be propagated through other paths, and this component should act
	// "silly" on it.
	if fetchErr != nil {
		// If we have stale cached data, return it as fallback
		if err == nil && cached.Tenant != nil {
			tm.logger.Warn().
				Err(fetchErr).
				Int64("tenantId", req.Id).
				Time("cachedValidUntil", cached.ValidUntil).
				Msg("API fetch failed, returning stale cached data")
			return cached.Tenant, nil
		}

		// No cached data available
		tm.logger.Error().
			Err(fetchErr).
			Int64("tenantId", req.Id).
			Msg("failed to retrieve remote tenant information and no cached data available")
		return nil, fetchErr
	}

	// Validate secret store configuration
	if tenant.SecretStore == nil {
		tm.logger.Warn().
			Int64("tenantId", req.Id).
			Msg("tenant retrieved from API without secret store details")
	} else if !isSecretStoreConfigured(tenant.SecretStore) {
		tm.logger.Warn().
			Int64("tenantId", req.Id).
			Msg("tenant retrieved from API with incomplete secret store configuration")
	}

	// Calculate expiration time using existing logic
	validUntil := tm.calculateValidUntil(tenant)

	// Create cached tenant
	newCached := cachedTenant{
		Tenant:     tenant,
		ValidUntil: validUntil,
		Modified:   tenant.Modified,
	}

	// Store in cache without TTL - we'll check ValidUntil manually
	// This allows us to return stale data if the API is unavailable
	if err := tm.cache.Set(ctx, key, newCached, 0); err != nil {
		tm.logger.Error().
			Err(err).
			Int64("tenantId", req.Id).
			Msg("failed to store tenant in cache")
		// Don't fail the request if cache storage fails
	}

	tm.logger.Debug().
		Int64("tenantId", req.Id).
		Time("validUntil", validUntil).
		Msg("tenant retrieved from API")

	return tenant, nil
}
