package secrets

import (
	"context"
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
	IsProtocolSecretsEnabled() bool
}

type TenantProvider interface {
	GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error)
}

// TenantSecrets provides backward compatibility with existing code
type TenantSecrets struct {
	tp                    TenantProvider
	logger                zerolog.Logger
	enableProtocolSecrets bool
}

// NewTenantSecrets creates a new TenantSecrets instance for backward compatibility
func NewTenantSecrets(tp TenantProvider, logger zerolog.Logger) *TenantSecrets {
	return &TenantSecrets{
		tp:                    tp,
		logger:                logger,
		enableProtocolSecrets: false, // Default to false for backward compatibility
	}
}

// GetSecretCredentials gets the secret store configuration for a tenant (backward compatibility)
func (ts *TenantSecrets) GetSecretCredentials(ctx context.Context, tenantID model.GlobalID) (*sm.SecretStore, error) {
	localTenantID, regionID := model.GetLocalAndRegionIDs(tenantID)
	ts.logger.Debug().
		Int("regionID", regionID).
		Int64("tenantId", localTenantID).
		Int64("globalTenantID", int64(tenantID)).
		Msg("getting secret credentials")

	tenant, err := ts.tp.GetTenant(ctx, &sm.TenantInfo{
		Id: int64(tenantID),
	})
	if err != nil {
		ts.logger.Warn().Err(err).Int64("tenantId", int64(tenantID)).Msg("failed to get tenant")
		return nil, err
	}

	ts.logger.Debug().
		Int64("tenantId", localTenantID).
		Bool("tenantHasSecretStore", tenant.SecretStore != nil).
		Msg("tenant retrieved for secret credentials")

	if tenant.SecretStore != nil {
		ts.logger.Debug().
			Int64("tenantId", localTenantID).
			Str("secretStoreUrl", tenant.SecretStore.Url).
			Bool("hasSecretStoreToken", tenant.SecretStore.Token != "").
			Float64("secretStoreExpiry", tenant.SecretStore.Expiry).
			Msg("secret store configuration retrieved successfully")
	}

	return tenant.SecretStore, nil
}

// GetSecretValue implements SecretProvider interface (backward compatibility)
func (ts *TenantSecrets) GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
	// For backward compatibility, return empty string
	// This will be replaced by the full implementation in PR 2
	return "", nil
}

// IsProtocolSecretsEnabled returns whether protocol secrets are enabled for this probe
func (ts *TenantSecrets) IsProtocolSecretsEnabled() bool {
	return ts.enableProtocolSecrets
}

// UpdateCapabilities updates the probe capabilities
func (ts *TenantSecrets) UpdateCapabilities(probeCapabilities *sm.Probe_Capabilities) {
	ts.enableProtocolSecrets = false
	if probeCapabilities != nil {
		ts.enableProtocolSecrets = probeCapabilities.EnableProtocolSecrets
	}
}

// secretProvider provides caching for secret values with TTL and intelligent response handling
type secretProvider struct {
	tenantProvider        TenantProvider
	cache                 *cache.Cache
	logger                zerolog.Logger
	enableProtocolSecrets bool
	gsmClientFactory      *GSMClientFactory
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
		tenantProvider:        tenantProvider,
		cache:                 cache.New(ttl, cleanupInterval),
		logger:                logger.With().Str("component", "secret-cache").Logger(),
		enableProtocolSecrets: false, // Default to false
		gsmClientFactory:      NewGSMClientFactory(),
	}
}

// NewSecretProviderWithCapabilities creates a new secret provider with probe capabilities
func NewSecretProviderWithCapabilities(tenantProvider TenantProvider, ttl time.Duration, logger zerolog.Logger, probeCapabilities *sm.Probe_Capabilities) SecretProvider {
	enableProtocolSecrets := false
	if probeCapabilities != nil {
		enableProtocolSecrets = probeCapabilities.EnableProtocolSecrets
	}

	// go-cache handles cleanup automatically, so we don't need manual cleanup
	// The cleanup interval is set to ttl/10 to ensure expired items are cleaned up reasonably quickly
	cleanupInterval := ttl / 10
	if cleanupInterval < time.Minute {
		cleanupInterval = time.Minute
	}

	return &secretProvider{
		tenantProvider:        tenantProvider,
		cache:                 cache.New(ttl, cleanupInterval),
		logger:                logger.With().Str("component", "secret-cache").Logger(),
		enableProtocolSecrets: enableProtocolSecrets,
		gsmClientFactory:      NewGSMClientFactory(),
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
		return "", fmt.Errorf("failed to get secret store credentials: %w", err)
	}

	if secretStore == nil {
		return "", fmt.Errorf("no secret store configured for tenant %d", tenantID)
	}

	// Create GSM client
	client, err := sp.gsmClientFactory.CreateClient(secretStore.Url, secretStore.Token)
	if err != nil {
		return "", fmt.Errorf("failed to create GSM client: %w", err)
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
		return "", fmt.Errorf("failed to contact GSM for secret '%s': %w", secretKey, err)
	}

	// Handle different status codes
	switch resp.StatusCode() {
	case http.StatusOK:
		// Success - update cache and return value
		if resp.JSON200 == nil {
			return "", fmt.Errorf("empty response from GSM for secret %s", secretKey)
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

		return "", fmt.Errorf("secret '%s' not found in GSM (404)", secretKey)

	case http.StatusUnauthorized:
		// Auth issue - remove from cache (credentials may have changed)
		sp.cache.Delete(cacheKey)

		sp.logger.Warn().
			Int64("tenantId", int64(tenantID)).
			Str("secretKey", secretKey).
			Msg("unauthorized accessing secret in GSM, removed from cache")

		return "", fmt.Errorf("unauthorized to access secret '%s' in GSM (401)", secretKey)

	default:
		// 5xx or other errors - leave cache unchanged
		statusCode := resp.StatusCode()

		sp.logger.Warn().
			Int("statusCode", statusCode).
			Int64("tenantId", int64(tenantID)).
			Str("secretKey", secretKey).
			Msg("GSM returned error status, leaving cache unchanged")

		return "", fmt.Errorf("GSM returned status %d for secret '%s'", statusCode, secretKey)
	}
}

// IsProtocolSecretsEnabled returns whether protocol secrets are enabled for this probe
func (sp *secretProvider) IsProtocolSecretsEnabled() bool {
	return sp.enableProtocolSecrets
}

// UpdateCapabilities updates the probe capabilities
func (sp *secretProvider) UpdateCapabilities(probeCapabilities *sm.Probe_Capabilities) {
	sp.enableProtocolSecrets = false
	if probeCapabilities != nil {
		sp.enableProtocolSecrets = probeCapabilities.EnableProtocolSecrets
	}
}
