package secrets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type SecretProvider interface {
	GetSecretCredentials(ctx context.Context, tenantID model.GlobalID) (*sm.SecretStore, error)
	GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error)
}

type TenantProvider interface {
	GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error)
}

// secretProvider provides caching for secret values with TTL and intelligent response handling
type secretProvider struct {
	tenantProvider   TenantProvider
	cache            *cache.Cache
	logger           zerolog.Logger
	gsmClientFactory *GSMClientFactory
}

// NewSecretProvider creates a new secret provider
func NewSecretProvider(tenantProvider TenantProvider, ttl time.Duration, logger zerolog.Logger) SecretProvider {
	// go-cache handles cleanup automatically, so we don't need manual cleanup
	// The cleanup interval is set to ttl/10 to ensure expired items are cleaned up reasonably quickly
	cleanupInterval := ttl / 10
	if cleanupInterval < time.Minute {
		cleanupInterval = time.Minute
	}

	return &secretProvider{
		tenantProvider:   tenantProvider,
		cache:            cache.New(ttl, cleanupInterval),
		logger:           logger.With().Str("component", "secret-cache").Logger(),
		gsmClientFactory: NewGSMClientFactory(),
	}
}

// Close gracefully shuts down the secret provider
func (sp *secretProvider) Close() {
	// go-cache doesn't require explicit cleanup, but we can flush the cache
	sp.cache.Flush()
}

// cacheKey creates a unique key for tenant+secret combination
func (sp *secretProvider) cacheKey(tenantID model.GlobalID, secretKey string) string {
	return fmt.Sprintf("%d:%s", tenantID, secretKey)
}

// GetSecretCredentials gets the secret store configuration for a tenant
func (sp *secretProvider) GetSecretCredentials(ctx context.Context, tenantID model.GlobalID) (*sm.SecretStore, error) {
	if sp.logger.GetLevel() <= zerolog.DebugLevel {
		tenantID, regionID := model.GetLocalAndRegionIDs(tenantID)
		sp.logger.Debug().Int("regionID", regionID).Int64("tenantId", tenantID).Msg("getting secret credentials")
	}

	tenant, err := sp.tenantProvider.GetTenant(ctx, &sm.TenantInfo{
		Id: int64(tenantID),
	})
	if err != nil {
		sp.logger.Warn().Err(err).Int64("tenantId", int64(tenantID)).Msg("failed to get tenant")
		return nil, err
	}

	return tenant.SecretStore, nil
}

// GetSecretValue implements caching with intelligent GSM response handling
func (sp *secretProvider) GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
	cacheKey := sp.cacheKey(tenantID, secretKey)

	// Check cache first
	if cachedValue, found := sp.cache.Get(cacheKey); found {
		sp.logger.Debug().
			Int64("tenantId", int64(tenantID)).
			Str("secretKey", secretKey).
			Msg("secret cache hit")

		return cachedValue.(string), nil
	}

	sp.logger.Debug().
		Int64("tenantId", int64(tenantID)).
		Str("secretKey", secretKey).
		Msg("secret cache miss, fetching from GSM")

	if sp.logger.GetLevel() <= zerolog.DebugLevel {
		tenantID, regionID := model.GetLocalAndRegionIDs(tenantID)
		sp.logger.Debug().Int("regionID", regionID).Int64("tenantId", tenantID).Str("secretKey", secretKey).Msg("getting secret value")
	}

	// Get the secret store configuration for this tenant
	secretStore, err := sp.GetSecretCredentials(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("could not load the secret store configuration: %w", err)
	}

	if secretStore == nil {
		return "", errors.New("no secret store is configured for this stack")
	}

	// Create GSM client
	client, err := sp.gsmClientFactory.CreateClient(secretStore.Url, secretStore.Token)
	if err != nil {
		return "", fmt.Errorf("could not connect to the secret store: %w", err)
	}

	// Get the decrypted secret value
	resp, err := client.DecryptSecretByIdWithResponse(ctx, secretKey)
	if err != nil {
		// Network error or client error - leave cache unchanged
		sp.logger.Warn().
			Err(err).
			Int64("tenantId", int64(tenantID)).
			Str("secretKey", secretKey).
			Msg("network error fetching secret, leaving cache unchanged")

		return "", fmt.Errorf("could not reach the secret store: %w", err)
	}

	// Handle different status codes
	switch resp.StatusCode() {
	case http.StatusOK:
		// Success - update cache and return value
		if resp.JSON200 == nil {
			return "", errors.New("the secret store returned an empty value")
		}

		secretValue := resp.JSON200.Plaintext
		sp.cache.Set(cacheKey, secretValue, cache.DefaultExpiration)

		sp.logger.Debug().
			Int64("tenantId", int64(tenantID)).
			Str("secretKey", secretKey).
			Msg("secret fetched from GSM and cached")

		return secretValue, nil

	case http.StatusNotFound:
		// Secret not found - remove from cache
		sp.cache.Delete(cacheKey)

		sp.logger.Warn().
			Int64("tenantId", int64(tenantID)).
			Str("secretKey", secretKey).
			Msg("secret not found in GSM, removed from cache")

		return "", errors.New("the secret does not exist")

	case http.StatusUnauthorized:
		// Auth issue - remove from cache (credentials may have changed)
		sp.cache.Delete(cacheKey)

		sp.logger.Warn().
			Int64("tenantId", int64(tenantID)).
			Str("secretKey", secretKey).
			Msg("unauthorized accessing secret in GSM, removed from cache")

		return "", errors.New("not authorized to read this secret")

	default:
		// 5xx or other errors - leave cache unchanged
		statusCode := resp.StatusCode()

		sp.logger.Warn().
			Int("statusCode", statusCode).
			Int64("tenantId", int64(tenantID)).
			Str("secretKey", secretKey).
			Msg("GSM returned error status, leaving cache unchanged")

		return "", fmt.Errorf("the secret store returned status %d", statusCode)
	}
}
