package tracing

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
)

func TestSetupDisabled(t *testing.T) {
	before := otel.GetTracerProvider()

	shutdown, err := Setup(context.Background(), Config{})
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	require.NoError(t, shutdown(context.Background()))

	require.Same(t, before, otel.GetTracerProvider(), "an empty endpoint must leave tracing disabled")
}

func TestSetupExportsSpans(t *testing.T) {
	collector := runCollector(t)

	// Setup replaces the global tracer provider, so put back whatever was there when we are done.
	before := otel.GetTracerProvider()

	t.Cleanup(func() { otel.SetTracerProvider(before) })

	shutdown, err := Setup(context.Background(), Config{
		Endpoint: collector.addr,
		Insecure: true,
		Version:  "v0.0.0-test",
	})
	require.NoError(t, err)

	provider, isSDK := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	require.True(t, isSDK, "a configured endpoint must install a real tracer provider")
	require.NotNil(t, provider)

	_, span := otel.Tracer("test").Start(context.Background(), "sm-k6")
	span.End()

	// Shutting down flushes the batcher, so by the time it returns the collector must have seen the span.
	require.NoError(t, shutdown(context.Background()))

	spans := collector.spans()
	require.Len(t, spans, 1)
	require.Equal(t, "sm-k6", spans[0].GetName())
}

// testCollector is a minimal OTLP/gRPC trace receiver that records everything it is sent.
type testCollector struct {
	coltracepb.UnimplementedTraceServiceServer

	addr string

	mu       sync.Mutex
	received []*tracepb.Span
}

func (c *testCollector) Export(_ context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, rs := range req.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			c.received = append(c.received, ss.GetSpans()...)
		}
	}

	return &coltracepb.ExportTraceServiceResponse{}, nil
}

func (c *testCollector) spans() []*tracepb.Span {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.received
}

// runCollector starts a testCollector on a random local port, stopping it when the test ends.
func runCollector(t *testing.T) *testCollector {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	collector := &testCollector{addr: listener.Addr().String()}

	server := grpc.NewServer()
	coltracepb.RegisterTraceServiceServer(server, collector)

	done := make(chan struct{})

	go func() {
		defer close(done)

		_ = server.Serve(listener)
	}()

	t.Cleanup(func() {
		server.Stop()
		<-done
	})

	return collector
}
