package k6runner

import (
	"slices"
	"strings"
)

// k6Env returns the environment variables that are passed to the k6 process that runs checks.
// Ideally, this should be a clean slate env, but we know people are relying on the fact that k6 inherits the agent's
// environment.
// TODO: Make this a clean slate on the next major release, as a breaking change.
func k6Env(localEnv []string) []string {
	// Set K6_BROWSER_LOG=info if it is not set already.
	if !slices.ContainsFunc(localEnv, func(e string) bool { return strings.HasPrefix(e, "K6_BROWSER_LOG=") }) {
		localEnv = append(localEnv, "K6_BROWSER_LOG=info")
	}

	return localEnv
}
