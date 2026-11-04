package k6runner

import (
	"fmt"
	"sync/atomic"
)

// HTTP debug modes, matching k6's --http-debug option.
const (
	HTTPDebugOff     = "off"
	HTTPDebugHeaders = "headers"
	HTTPDebugFull    = "full"
)

// HTTPDebug holds the k6 --http-debug mode for local runs. It can be changed
// at any time: each run reads the current value when it builds the k6
// arguments. The zero value is off.
//
// "full" includes request and response bodies in the check logs, which may
// contain credentials.
type HTTPDebug struct {
	mode atomic.Value // string
}

// Set changes the mode. An empty mode is the same as off.
func (d *HTTPDebug) Set(mode string) error {
	switch mode {
	case "":
		mode = HTTPDebugOff
	case HTTPDebugOff, HTTPDebugHeaders, HTTPDebugFull:
	default:
		return fmt.Errorf("invalid k6 HTTP debug mode %q: must be off, headers or full", mode)
	}

	d.mode.Store(mode)

	return nil
}

// Get returns the current mode.
func (d *HTTPDebug) Get() string {
	if d == nil {
		return HTTPDebugOff
	}

	mode, _ := d.mode.Load().(string)
	if mode == "" {
		return HTTPDebugOff
	}

	return mode
}

// args returns the k6 arguments for the current mode.
func (d *HTTPDebug) args() []string {
	switch d.Get() {
	case HTTPDebugHeaders:
		return []string{"--http-debug"}
	case HTTPDebugFull:
		return []string{"--http-debug=full"}
	default:
		return nil
	}
}
