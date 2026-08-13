package k6runner

import (
	"context"
	"slices"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestEnv(t *testing.T) {
	t.Parallel()

	t.Run("adds default variables", func(t *testing.T) {
		t.Parallel()

		osenv := []string{"SOMETHING=else", "FOO=bar"}
		modified := k6Env(osenv)

		if !slices.Contains(modified, "K6_BROWSER_LOG=info") {
			t.Fatalf("Expected env to contain browser log info")
		}

		if !slices.Contains(modified, "K6_AUTO_EXTENSION_RESOLUTION=false") {
			t.Fatalf("Expected env to contain K6_AUTO_EXTENSION_RESOLUTION")
		}
	})

	t.Run("does not modify existing value", func(t *testing.T) {
		t.Parallel()

		osenv := []string{"SOMETHING=else", "FOO=bar", "K6_BROWSER_LOG=debug", "K6_AUTO_EXTENSION_RESOLUTION=true"}
		modified := k6Env(osenv)

		if !slices.Contains(modified, "K6_BROWSER_LOG=debug") {
			t.Fatalf("Expected env to contain original variable")
		}

		if !slices.Contains(modified, "K6_AUTO_EXTENSION_RESOLUTION=true") {
			t.Fatalf("Expected env to contain original variable")
		}

		if slices.Contains(modified, "K6_BROWSER_LOG=info") {
			t.Fatalf("Expected env to _not_ contain browser log info")
		}
	})
}

func TestBrowserTracesEnv(t *testing.T) {
	t.Parallel()

	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:  trace.SpanID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
	})

	for _, tc := range []struct {
		name     string
		ctx      context.Context //nolint:containedctx // Table-driven test over context contents.
		expected []string
	}{
		{
			name:     "no span",
			ctx:      context.Background(),
			expected: nil,
		},
		{
			name:     "invalid span context",
			ctx:      trace.ContextWithSpanContext(context.Background(), trace.SpanContext{}),
			expected: nil,
		},
		{
			name: "valid span context",
			ctx:  trace.ContextWithSpanContext(context.Background(), spanCtx),
			expected: []string{
				"K6_BROWSER_TRACES_METADATA=" +
					"sm.trace.id=0102030405060708090a0b0c0d0e0f10," +
					"sm.span.id=1112131415161718",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if actual := browserTracesEnv(tc.ctx); !slices.Equal(actual, tc.expected) {
				t.Fatalf("Expected %q, got %q", tc.expected, actual)
			}
		})
	}
}
