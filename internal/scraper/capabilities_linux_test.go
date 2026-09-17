package scraper

import (
	"testing"

	"kernel.org/pub/linux/libs/security/libcap/cap"
)

// requireTracerouteCapabilities skips the calling test unless the process holds the capabilities
// the traceroute prober needs in order to open raw sockets.
//
// The libcap import is confined to this Linux-only file on purpose. It pulls in C sources that
// cannot be compiled without cgo, and libcap itself is Linux-only, so importing it directly from
// scraper_test.go took down the entire package test binary on any other platform.
func requireTracerouteCapabilities(t *testing.T) {
	t.Helper()

	proc := cap.GetProc()

	for _, value := range []cap.Value{cap.NET_ADMIN, cap.NET_RAW} {
		permitted, err := proc.GetFlag(cap.Permitted, value)
		if err != nil {
			t.Fatalf("cannot get %s flag: %s", value, err)
		}

		if !permitted {
			t.Skipf("traceroute cannot run, process doesn't have %s capability", value)
		}
	}
}
