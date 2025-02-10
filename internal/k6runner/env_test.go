package k6runner

import (
	"slices"
	"testing"
)

func TestEnv(t *testing.T) {
	t.Parallel()

	t.Run("adds k6 browser log default", func(t *testing.T) {
		t.Parallel()

		osenv := []string{"SOMETHING=else", "FOO=bar"}
		modified := k6Env(osenv)

		if !slices.Contains(modified, "K6_BROWSER_LOG=info") {
			t.Fatalf("Expected env to contain browser log info")
		}
	})

	t.Run("does not modify existing value", func(t *testing.T) {
		t.Parallel()

		osenv := []string{"SOMETHING=else", "FOO=bar", "K6_BROWSER_LOG=debug"}
		modified := k6Env(osenv)

		if !slices.Contains(modified, "K6_BROWSER_LOG=debug") {
			t.Fatalf("Expected env to contain original variable")
		}

		if slices.Contains(modified, "K6_BROWSER_LOG=info") {
			t.Fatalf("Expected env to _not_ contain browser log info")
		}
	})
}
