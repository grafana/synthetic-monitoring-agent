package multihttp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

const proberName = "multihttp"

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

func NewProber(ctx context.Context, check model.Check, logger zerolog.Logger, runner k6runner.Runner, reservedHeaders http.Header, store secrets.SecretProvider) (Prober, error) {
	var p Prober

	if check.Settings.Multihttp == nil {
		return p, errUnsupportedCheck
	}

	if err := check.Settings.Multihttp.Validate(); err != nil {
		return p, err
	}

	if len(reservedHeaders) > 0 {
		augmentHttpHeaders(&check.Check, reservedHeaders)
	}

	script, err := settingsToScript(check.Settings.Multihttp)
	if err != nil {
		return p, err
	}

	p.module = Module{
		Prober: sm.CheckTypeMultiHttp.String(),
		Script: k6runner.Script{
			Script: script,
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

	logger.Debug().
		Int64("tenantId", check.TenantId).
		Int64("checkId", check.Id).
		Str("target", check.Target).
		Str("job", check.Job).
		Bytes("script", script).
		Msg("created prober")

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
		p.logger.Error().Err(err).Msg("running probe")
		return false, 0
	}

	success, duration, err := p.processor.Run(ctx, registry, logger, p.logger, secretStore)
	if err != nil {
		p.logger.Error().Err(err).Msg("running probe")
		return false, 0
	}

	return success, duration.Seconds()
}

// Overrides any user-provided headers with our own augmented values
// for 'reserved' headers.
func augmentHttpHeaders(check *sm.Check, reservedHeaders http.Header) {
	updatedHeaders := []*sm.HttpHeader{}
	for key, values := range reservedHeaders {
		updatedHeaders = append(updatedHeaders, &sm.HttpHeader{Name: key, Value: strings.Join(values, ",")})
	}

	for _, entry := range check.Settings.Multihttp.Entries {
		heads := entry.Request.Headers
		for _, headerPtr := range heads {
			_, present := reservedHeaders[http.CanonicalHeaderKey(headerPtr.Name)]

			if present {
				continue // users can't override reserved headers with their own values
			}

			updatedHeaders = append(updatedHeaders, headerPtr)
		}

		entry.Request.Headers = updatedHeaders
	}
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
