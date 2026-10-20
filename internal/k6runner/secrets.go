package k6runner

import (
	"context"

	"github.com/go-kit/log/level"
	smmodel "github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
	"github.com/rs/zerolog"
)

// SecretStoreRetriever loads the stack's secret store configuration.
// A failure reported to the check owner on logger.
type SecretStoreRetriever func(ctx context.Context, logger logger.Logger) (SecretStore, error)

// NewSecretStoreRetriever returns a SecretStoreRetriever that reads the configuration for tenantID from provider.
func NewSecretStoreRetriever(provider secrets.SecretProvider, tenantID smmodel.GlobalID, internalLogger zerolog.Logger) SecretStoreRetriever {
	return func(ctx context.Context, ownerLogger logger.Logger) (SecretStore, error) {
		var store SecretStore

		internalLogger.Debug().
			Int64("tenantId", int64(tenantID)).
			Msg("credentials retriever: getting secret credentials")

		credentials, err := provider.GetSecretCredentials(ctx, tenantID)
		if err != nil {
			internalLogger.Error().
				Err(err).
				Int64("tenantId", int64(tenantID)).
				Msg("credentials retriever: failed to get secret credentials")

			// The check owner gets a fixed reason instead of error because it
			// might contain the transport detail of the SM API client.
			_ = level.Error(ownerLogger).Log("msg", "Could not load the secret store configuration for this check", "reason", secrets.OwnerReason(err))

			return store, err
		}

		if credentials != nil {
			store = SecretStore{
				Url:   credentials.Url,
				Token: credentials.Token,
			}
			internalLogger.Debug().
				Int64("tenantId", int64(tenantID)).
				Str("secretStoreUrl", credentials.Url).
				Bool("hasSecretStoreToken", credentials.Token != "").
				Float64("secretStoreExpiry", credentials.Expiry).
				Msg("credentials retriever: secret store configured")
		} else {
			internalLogger.Debug().
				Int64("tenantId", int64(tenantID)).
				Msg("credentials retriever: no secret store configuration")
		}

		return store, nil
	}
}
