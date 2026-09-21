//go:build !linux

package scraper

import (
	"runtime"
	"testing"
)

// requireTracerouteCapabilities always skips on non-Linux platforms. Capabilities are a Linux
// concept, so there is nothing to probe for here - see the Linux variant of this file for why the
// libcap dependency has to stay out of the shared test files.
func requireTracerouteCapabilities(t *testing.T) {
	t.Helper()

	t.Skipf("traceroute capability check is not implemented on %s", runtime.GOOS)
}
