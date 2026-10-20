package secrets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

var (
	ErrNoSecretStore           = ownerReason("no secret store is configured for this stack")
	ErrSecretNotFound          = ownerReason("the secret does not exist")
	ErrSecretStoreUnavailable  = ownerReason("the secret store is unavailable")
	ErrSecretStoreUnauthorized = ownerReason("the secret store rejected the agent's access token")
	ErrSecretStoreBadResponse  = ownerReason("the secret store returned an unexpected response")
)

type ownerReasonError struct {
	reason string
}

func (e *ownerReasonError) Error() string { return e.reason }

func ownerReason(reason string) error {
	return &ownerReasonError{reason: reason}
}

// OwnerReason returns the reason why secret could not be fetched without exposing sensitive information.
func OwnerReason(err error) string {
	if reasonErr, ok := errors.AsType[*ownerReasonError](err); ok {
		return reasonErr.reason
	}

	return "the secret could not be fetched"
}

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
		return nil, fmt.Errorf("%w: could not load the secret store configuration: %w", ErrSecretStoreUnavailable, err)
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
		return "", err
	}

	if secretStore == nil {
		return "", ErrNoSecretStore
	}

	// Create GSM client
	client, err := sp.gsmClientFactory.CreateClient(secretStore.Url, secretStore.Token)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrSecretStoreUnavailable, err)
	}

	// Get the decrypted secret value
	resp, err := client.DecryptSecretByIdWithResponse(ctx, secretKey)
	if err != nil {
		// Network error or client error - leave cache unchanged
		reason, msg := ErrSecretStoreUnavailable, "network error fetching secret, leaving cache unchanged"

		// The HTTP client reports almost every transport failure as a *url.Error
		if _, ok := errors.AsType[*url.Error](err); !ok {
			reason, msg = ErrSecretStoreBadResponse, "unreadable response from GSM, leaving cache unchanged"
		}

		sp.logger.Warn().
			Err(err).
			Int64("tenantId", int64(tenantID)).
			Str("secretKey", secretKey).
			Msg(msg)

		return "", fmt.Errorf("%w: %w", reason, err)
	}

	// Handle different status codes
	switch resp.StatusCode() {
	case http.StatusOK:
		// Success - update cache and return value
		if resp.JSON200 == nil {
			return "", fmt.Errorf("%w: status 200 with content type %q", ErrSecretStoreBadResponse, resp.HTTPResponse.Header.Get("Content-Type"))
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

		return "", ErrSecretNotFound

	case http.StatusUnauthorized:
		// Auth issue - remove from cache (credentials may have changed)
		sp.cache.Delete(cacheKey)

		sp.logger.Warn().
			Int64("tenantId", int64(tenantID)).
			Str("secretKey", secretKey).
			Msg("unauthorized accessing secret in GSM, removed from cache")

		return "", ErrSecretStoreUnauthorized

	default:
		// 5xx or other errors - leave cache unchanged
		statusCode := resp.StatusCode()

		sp.logger.Warn().
			Int("statusCode", statusCode).
			Int64("tenantId", int64(tenantID)).
			Str("secretKey", secretKey).
			Msg("GSM returned error status, leaving cache unchanged")

		reason := ErrSecretStoreBadResponse
		if statusCode >= http.StatusInternalServerError {
			reason = ErrSecretStoreUnavailable
		}

		return "", fmt.Errorf("%w: status %d", reason, statusCode)
	}
}
