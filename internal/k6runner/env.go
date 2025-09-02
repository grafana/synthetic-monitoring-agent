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
