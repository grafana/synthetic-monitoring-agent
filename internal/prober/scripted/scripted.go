package scripted

import (
	"context"
	"errors"

	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

const proberName = "scripted"

var errUnsupportedCheck = errors.New("unsupported check")

type Module struct {
	Prober string
	Script k6runner.Script
}

type Prober struct {
	logger           zerolog.Logger
	module           Module
	processor        *k6runner.Processor
	secretsRetriever func(context.Context) (k6runner.SecretStore, error)
}

func NewProber(ctx context.Context, check model.Check, logger zerolog.Logger, runner k6runner.Runner, store secrets.SecretProvider) (Prober, error) {
	var p Prober

	if check.Settings.Scripted == nil {
		return p, errUnsupportedCheck
	}

	p.module = Module{
		Prober: sm.CheckTypeScripted.String(),
		Script: k6runner.Script{
			Script: check.Settings.Scripted.Script,
			Settings: k6runner.Settings{
				Timeout: check.Timeout,
			},
			CheckInfo: k6runner.CheckInfoFromSM(check),
		},
	}

	processor, err := k6runner.NewProcessor(p.module.Script, runner)
	if err != nil {
		return p, err
	}

	p.processor = processor
	p.logger = logger
	p.secretsRetriever = newCredentialsRetriever(store, check.GlobalTenantID(), p.logger)

	return p, nil
}

func (p Prober) Name() string {
	return proberName
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) (bool, float64) {
	secretStore, err := p.secretsRetriever(ctx)
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to retrieve secret store")
		return false, 0
	}

	p.logger.Debug().
		Str("target", target).
		Bool("secretStoreIsConfigured", secretStore.IsConfigured()).
		Str("secretStoreUrl", secretStore.Url).
		Bool("hasSecretStoreToken", secretStore.Token != "").
		Msg("secret store retrieved for scripted probe")

	success, duration, err := p.processor.Run(ctx, registry, logger, p.logger, secretStore)
	if err != nil {
		p.logger.Error().Err(err).Msg("running probe")
		return false, 0
	}

	return success, duration.Seconds()
}

func newCredentialsRetriever(provider secrets.SecretProvider, tenantID model.GlobalID, logger zerolog.Logger) func(context.Context) (k6runner.SecretStore, error) {
	return func(ctx context.Context) (k6runner.SecretStore, error) {
		var store k6runner.SecretStore

		logger.Debug().
			Int64("tenantId", int64(tenantID)).
			Msg("credentials retriever: getting secret credentials")

		credentials, err := provider.GetSecretCredentials(ctx, tenantID)
		if err != nil {
			logger.Error().
				Err(err).
				Int64("tenantId", int64(tenantID)).
				Msg("credentials retriever: failed to get secret credentials")
			return store, err
		}

		if credentials != nil {
			store = k6runner.SecretStore{
				Url:   credentials.Url,
				Token: credentials.Token,
			}
			logger.Debug().
				Int64("tenantId", int64(tenantID)).
				Str("secretStoreUrl", credentials.Url).
				Bool("hasSecretStoreToken", credentials.Token != "").
				Float64("secretStoreExpiry", credentials.Expiry).
				Msg("credentials retriever: secret store configured")
		} else {
			logger.Debug().
				Int64("tenantId", int64(tenantID)).
				Msg("credentials retriever: no secret store configuration")
		}

		return store, nil
	}
}
