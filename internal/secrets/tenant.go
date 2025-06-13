package secrets

import (
	"context"
	"fmt"
	"net/http"

	gsmClient "github.com/grafana/gsm-api-go-client"
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

type TenantSecrets struct {
	tp     TenantProvider
	logger zerolog.Logger
}

func NewTenantSecrets(tp TenantProvider, logger zerolog.Logger) *TenantSecrets {
	return &TenantSecrets{
		tp:     tp,
		logger: logger,
	}
}

func (ts *TenantSecrets) GetSecretCredentials(ctx context.Context, tenantID model.GlobalID) (*sm.SecretStore, error) {
	if ts.logger.GetLevel() <= zerolog.DebugLevel {
		tenantID, regionID := model.GetLocalAndRegionIDs(tenantID)
		ts.logger.Debug().Int("regionID", regionID).Int64("tenantId", tenantID).Msg("getting secret credentials")
	}

	tenant, err := ts.tp.GetTenant(ctx, &sm.TenantInfo{
		Id: int64(tenantID),
	})
	if err != nil {
		ts.logger.Warn().Err(err).Int64("tenantId", int64(tenantID)).Msg("failed to get tenant")
		return nil, err
	}

	return tenant.SecretStore, nil
}

func (ts *TenantSecrets) GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
	if ts.logger.GetLevel() <= zerolog.DebugLevel {
		tenantID, regionID := model.GetLocalAndRegionIDs(tenantID)
		ts.logger.Debug().Int("regionID", regionID).Int64("tenantId", tenantID).Str("secretKey", secretKey).Msg("getting secret value")
	}

	// Get the secret store configuration for this tenant
	secretStore, err := ts.GetSecretCredentials(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("failed to get secret store credentials: %w", err)
	}

	if secretStore == nil {
		return "", fmt.Errorf("no secret store configured for tenant %d", tenantID)
	}

	// Create GSM client
	client, err := ts.createGSMClient(secretStore.Url, secretStore.Token)
	if err != nil {
		return "", fmt.Errorf("failed to create GSM client: %w", err)
	}

	// Get the decrypted secret value
	resp, err := client.DecryptSecretByIdWithResponse(ctx, secretKey)
	if err != nil {
		ts.logger.Warn().Err(err).Int64("tenantId", int64(tenantID)).Str("secretKey", secretKey).Msg("failed to get secret from GSM")
		return "", fmt.Errorf("failed to get secret from GSM: %w", err)
	}

	// Check response status
	if resp.StatusCode() != http.StatusOK {
		ts.logger.Warn().Int("statusCode", resp.StatusCode()).Int64("tenantId", int64(tenantID)).Str("secretKey", secretKey).Msg("GSM returned non-200 status")
		return "", fmt.Errorf("GSM returned status %d for secret %s", resp.StatusCode(), secretKey)
	}

	if resp.JSON200 == nil {
		return "", fmt.Errorf("empty response from GSM for secret %s", secretKey)
	}

	// Return the plaintext value
	return resp.JSON200.Plaintext, nil
}

// createGSMClient creates a new GSM client with the provided URL and token
func (ts *TenantSecrets) createGSMClient(url, token string) (gsmClient.ClientWithResponsesInterface, error) {
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
