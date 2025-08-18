package browser

import (
	"context"
	"errors"

	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

const proberName = "browser"

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

	if check.Settings.Browser == nil {
		return p, errUnsupportedCheck
	}

	p.module = Module{
		Prober: sm.CheckTypeBrowser.String(),
		Script: k6runner.Script{
			Script: check.Settings.Browser.Script,
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
	p.secretsRetriever = newCredentialsRetriever(store, check.GlobalTenantID())

	return p, nil
}

func (p Prober) Name() string {
	return proberName
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, zlogger zerolog.Logger) (bool, float64) {
	secretStore, err := p.secretsRetriever(ctx)

	if err != nil {
		zlogger.Error().Err(err).Msg("running probe")
		return false, 0
	}

	success, err := p.processor.Run(ctx, registry, zlogger, secretStore)
	if err != nil {
		zlogger.Error().Err(err).Msg("running probe")
		return false, 0
	}

	// TODO(mem): implement custom duration extraction.
	return success, 0
}

// TODO(mem): This should probably be in the k6runner package.
func newCredentialsRetriever(provider secrets.SecretProvider, tenantID model.GlobalID) func(context.Context) (k6runner.SecretStore, error) {
	return func(ctx context.Context) (k6runner.SecretStore, error) {
		var store k6runner.SecretStore

		credentials, err := provider.GetSecretCredentials(ctx, tenantID)
		if err != nil {
			return store, err
		}

		if credentials != nil {
			store = k6runner.SecretStore{
				Url:   credentials.Url,
				Token: credentials.Token,
			}
		}

		return store, nil
	}
}
