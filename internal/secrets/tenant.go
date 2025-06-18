package secrets

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gsmClient "github.com/grafana/gsm-api-go-client"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type SecretProvider interface {
	GetSecretCredentials(ctx context.Context, tenantID model.GlobalID) (*sm.SecretStore, error)
	GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error)
}

// CapabilityAwareSecretProvider extends SecretProvider with probe capability awareness
type CapabilityAwareSecretProvider interface {
	SecretProvider
	IsProtocolSecretsEnabled() bool
}

// UpdatableCapabilityAwareSecretProvider allows updating probe capabilities after creation
type UpdatableCapabilityAwareSecretProvider interface {
	CapabilityAwareSecretProvider
	UpdateCapabilities(probeCapabilities *sm.Probe_Capabilities)
}

type TenantProvider interface {
	GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error)
}

// capabilityAwareWrapper wraps a SecretProvider to add probe capability awareness
type capabilityAwareWrapper struct {
	SecretProvider
	enableProtocolSecrets bool
}

// updatableCapabilityAwareWrapper wraps a SecretProvider to add updatable probe capability awareness
type updatableCapabilityAwareWrapper struct {
	SecretProvider
	enableProtocolSecrets bool
}

// NewCapabilityAwareSecretProvider wraps a SecretProvider with probe capabilities
func NewCapabilityAwareSecretProvider(secretProvider SecretProvider, probeCapabilities *sm.Probe_Capabilities) CapabilityAwareSecretProvider {
	enableProtocolSecrets := false
	if probeCapabilities != nil {
		enableProtocolSecrets = probeCapabilities.EnableProtocolSecrets
	}

	return &capabilityAwareWrapper{
		SecretProvider:        secretProvider,
		enableProtocolSecrets: enableProtocolSecrets,
	}
}

// NewUpdatableCapabilityAwareSecretProvider wraps a SecretProvider with updatable probe capabilities
func NewUpdatableCapabilityAwareSecretProvider(secretProvider SecretProvider) UpdatableCapabilityAwareSecretProvider {
	return &updatableCapabilityAwareWrapper{
		SecretProvider:        secretProvider,
		enableProtocolSecrets: false, // Default to false until capabilities are set
	}
}

// IsProtocolSecretsEnabled returns whether protocol secrets are enabled for this probe
func (w *capabilityAwareWrapper) IsProtocolSecretsEnabled() bool {
	return w.enableProtocolSecrets
}

// IsProtocolSecretsEnabled returns whether protocol secrets are enabled for this probe
func (w *updatableCapabilityAwareWrapper) IsProtocolSecretsEnabled() bool {
	return w.enableProtocolSecrets
}

// UpdateCapabilities updates the probe capabilities
func (w *updatableCapabilityAwareWrapper) UpdateCapabilities(probeCapabilities *sm.Probe_Capabilities) {
	w.enableProtocolSecrets = false
	if probeCapabilities != nil {
		w.enableProtocolSecrets = probeCapabilities.EnableProtocolSecrets
	}
}

// secretProvider provides caching for secret values with TTL and intelligent response handling
type secretProvider struct {
	tenantProvider TenantProvider
	cache          *cache.Cache
	logger         zerolog.Logger
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
		tenantProvider: tenantProvider,
		cache:          cache.New(ttl, cleanupInterval),
		logger:         logger.With().Str("component", "secret-cache").Logger(),
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
	client, err := sp.createGSMClient(secretStore.Url, secretStore.Token)
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

// createGSMClient creates a new GSM client with the provided URL and token
func (sp *secretProvider) createGSMClient(url, token string) (gsmClient.ClientWithResponsesInterface, error) {
	if url == "" {
		return nil, fmt.Errorf("GSM URL cannot be empty")
	}
	if token == "" {
		return nil, fmt.Errorf("GSM token cannot be empty")
	}
	return gsmClient.NewClientWithResponses(url, withAuth(token), withAcceptJSON())
}

// withAuth adds the Authorization header with Bearer token
func withAuth(token string) gsmClient.ClientOption {
	return gsmClient.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Add("Authorization", "Bearer "+token)
		return nil
	})
}

// withAcceptJSON adds the Accept: application/json header
func withAcceptJSON() gsmClient.ClientOption {
	return gsmClient.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Add("Accept", "application/json")
		return nil
	})
}
