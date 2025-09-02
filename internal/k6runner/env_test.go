package k6runner

import (
	"slices"
	"testing"
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
