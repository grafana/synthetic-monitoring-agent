package k6runner

import (
	"context"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// k6Env returns the environment variables that are passed to the k6 process that runs checks.
// Ideally, this should be a clean slate env, but we know people are relying on the fact that k6 inherits the agent's
// environment.
// TODO: Make this a clean slate on the next major release, as a breaking change.
func k6Env(localEnv []string) []string {
	// envDefaults maps environment variables to their value. They will be set only if the environment variable is not
	// already present on localEnv.
	envDefaults := map[string]string{
		"K6_BROWSER_LOG":               "info",
		"K6_AUTO_EXTENSION_RESOLUTION": "false",
	}

	for env, val := range envDefaults {
		if !slices.ContainsFunc(localEnv, func(e string) bool { return strings.HasPrefix(e, env+"=") }) {
			localEnv = append(localEnv, env+"="+val)
		}
	}

	return localEnv
}

// browserTracesEnv returns a K6_BROWSER_TRACES_METADATA entry that stamps the identity of the span covering this
// execution onto every span k6's browser module emits, or nothing at all if ctx carries no valid span, which is the
// case whenever tracing is disabled.
//
// k6 has no way to adopt a parent span handed to it from the outside, so its traces are always rooted at their own
// iteration span. Carrying our trace and span IDs as attributes is what allows an operator to pivot between the agent
// span for a check execution and the browser trace for that same execution.
//
// Note that k6 only emits these traces when it is told where to send them, via K6_TRACES_OUTPUT or --traces-output,
// which k6 inherits from the agent's own environment.
func browserTracesEnv(ctx context.Context) []string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return nil
	}

	return []string{
		"K6_BROWSER_TRACES_METADATA=" +
			"sm.trace.id=" + spanCtx.TraceID().String() +
			",sm.span.id=" + spanCtx.SpanID().String(),
	}
}
