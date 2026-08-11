// Package tracing sets up OpenTelemetry trace export for the agent.
//
// Tracing is opt-in: unless an endpoint is configured, the global tracer provider is left alone, which means the
// instrumentation sprinkled around the agent resolves to no-ops and costs close to nothing.
package tracing

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// ServiceName is the value reported as service.name for the spans emitted by the agent. Note that collectors sitting
// between the agent and the tracing backend may rewrite this.
const ServiceName = "synthetic-monitoring-agent"

// Config holds the trace export configuration.
type Config struct {
	// Endpoint is the OTLP/gRPC endpoint spans are sent to, e.g. "localhost:4319". If empty, tracing is disabled.
	Endpoint string
	// Insecure disables transport security when talking to Endpoint. Needed for local collectors that do not serve
	// TLS.
	Insecure bool
	// Version is reported as service.version.
	Version string
}

// Setup configures the global tracer provider according to cfg, and returns a function that flushes pending spans and
// releases the associated resources.
//
// If cfg.Endpoint is empty, tracing is left disabled and the returned function is a no-op.
func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }

	if cfg.Endpoint == "" {
		return noop, nil
	}

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	// The exporter connects lazily, so an unreachable collector does not prevent the agent from starting.
	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return noop, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	// NewSchemaless, rather than NewWithAttributes, on purpose: resource.Merge refuses to merge resources with
	// conflicting schema URLs, and the one used by resource.Default() moves with every otel SDK bump. A schemaless
	// resource merges with any of them, so upgrading the SDK cannot break this at runtime.
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName(ServiceName),
			semconv.ServiceVersion(cfg.Version),
		),
	)
	if err != nil {
		return noop, errors.Join(fmt.Errorf("building trace resource: %w", err), exporter.Shutdown(ctx))
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(provider)

	return provider.Shutdown, nil
}
