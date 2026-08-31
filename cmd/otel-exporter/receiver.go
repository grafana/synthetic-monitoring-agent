package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"go.opentelemetry.io/collector/client"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// buildRouter returns a consumer function that demultiplexes incoming
// traces by tenant: it reads the bearer token from the request's
// Authorization header (propagated into ctx by the receiver's
// IncludeMetadata setting), matches it against known tenant tokens, and
// forwards the whole, unmodified request to that tenant's exporter — a
// request authenticates as exactly one tenant, so there's nothing to
// split or strip.
func buildRouter(exporters map[string]exporter.Traces, tokenToTenant map[string]string) consumer.ConsumeTracesFunc {
	return func(ctx context.Context, td ptrace.Traces) error {
		authHeaders := client.FromContext(ctx).Metadata.Get("Authorization")
		if len(authHeaders) == 0 {
			return status.Error(codes.Unauthenticated, "missing authorization header")
		}
		token := strings.TrimPrefix(authHeaders[0], "Bearer ")

		tenantName, known := tokenToTenant[token]
		if !known {
			return status.Error(codes.Unauthenticated, "unknown token")
		}

		texp, ok := exporters[tenantName]
		if !ok {
			return status.Errorf(codes.Internal, "no exporter configured for tenant %q", tenantName)
		}

		slog.Info("received data", "tenant", tenantName)

		if err := texp.ConsumeTraces(ctx, td); err != nil {
			return fmt.Errorf("tenant %q: %w", tenantName, err)
		}
		return nil
	}
}

// buildReceiver creates and starts an HTTP-only OTLP receiver on
// listenAddr, forwarding everything it receives into next.
func buildReceiver(ctx context.Context, listenAddr string, next consumer.Traces) (receiver.Traces, error) {
	factory := otlpreceiver.NewFactory()

	httpServerCfg := confighttp.NewDefaultServerConfig()
	httpServerCfg.NetAddr.Endpoint = listenAddr
	// Propagates incoming HTTP headers (Authorization, in particular) into
	// the context the consumer callback sees via client.FromContext.
	httpServerCfg.IncludeMetadata = true

	cfg := factory.CreateDefaultConfig().(*otlpreceiver.Config)
	cfg.Protocols.GRPC = configoptional.None[configgrpc.ServerConfig]()
	cfg.Protocols.HTTP = configoptional.Some(otlpreceiver.HTTPConfig{
		ServerConfig:  httpServerCfg,
		TracesURLPath: "/v1/traces",
	})

	telemetrySettings := componenttest.NewNopTelemetrySettings()
	telemetrySettings.Logger = zap.New(NewSlogCore(slog.Default().Handler(), zap.DebugLevel)).Named("collector")

	settings := receiver.Settings{
		ID:                component.NewID(factory.Type()),
		TelemetrySettings: telemetrySettings,
		BuildInfo:         component.NewDefaultBuildInfo(),
	}

	rcvr, err := factory.CreateTraces(ctx, settings, cfg, next)
	if err != nil {
		return nil, fmt.Errorf("creating receiver: %w", err)
	}
	if err := rcvr.Start(ctx, componenttest.NewNopHost()); err != nil {
		return nil, fmt.Errorf("starting receiver: %w", err)
	}
	return rcvr, nil
}
