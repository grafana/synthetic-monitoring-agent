package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.uber.org/zap"
)

// buildTenantExporter creates and starts a dedicated otlphttpexporter
// instance for a tenant, authenticated with its basic-auth credentials.
func buildTenantExporter(ctx context.Context, factory exporter.Factory, tenant Tenant) (exporter.Traces, error) {
	cfg := factory.CreateDefaultConfig().(*otlphttpexporter.Config)
	cfg.ClientConfig.Endpoint = tenant.Endpoint

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(tenant.Username+":"+tenant.Password))
	cfg.ClientConfig.Headers.Set("Authorization", configopaque.String(auth))

	// Disable the default sending queue so ConsumeTraces is synchronous:
	// its return value (and thus the HTTP response given back to whoever
	// routed a trace to us) should reflect whether Tempo actually accepted
	// it, not just whether it was queued locally.
	cfg.QueueConfig = configoptional.None[exporterhelper.QueueBatchConfig]()

	telemetrySettings := componenttest.NewNopTelemetrySettings()
	telemetrySettings.Logger = zap.New(NewSlogCore(slog.Default().Handler(), zap.DebugLevel)).Named(tenant.Name)

	settings := exporter.Settings{
		ID:                component.NewIDWithName(factory.Type(), tenant.Name),
		TelemetrySettings: telemetrySettings,
		BuildInfo:         component.NewDefaultBuildInfo(),
	}

	texp, err := factory.CreateTraces(ctx, settings, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating exporter: %w", err)
	}
	if err := texp.Start(ctx, componenttest.NewNopHost()); err != nil {
		return nil, fmt.Errorf("starting exporter: %w", err)
	}
	return texp, nil
}
