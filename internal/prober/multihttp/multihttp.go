package multihttp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
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
	logger    zerolog.Logger
	module    Module
	processor *k6runner.Processor
}

func NewProber(ctx context.Context, check sm.Check, logger zerolog.Logger, runner k6runner.Runner, reservedHeaders http.Header) (Prober, error) {
	var p Prober

	if check.Settings.Multihttp == nil {
		return p, errUnsupportedCheck
	}

	if err := check.Settings.Multihttp.Validate(); err != nil {
		return p, err
	}

	if len(reservedHeaders) > 0 {
		augmentHttpHeaders(&check, reservedHeaders)
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
			// TODO: Add metadata & features here.
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

	return p, nil
}

func (p Prober) Name() string {
	return proberName
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	success, err := p.processor.Run(ctx, registry, logger, p.logger)
	if err != nil {
		p.logger.Error().Err(err).Msg("running probe")
		return false
	}

	return success
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
