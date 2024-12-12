package k6runner

import (
	"bytes"
	"errors"
	"strings"

	"github.com/go-logfmt/logfmt"
	"golang.org/x/exp/maps"
)

var (
	ErrStacktrace = errors.New("fatal error occurred while running the script")
	ErrThrown     = errors.New("uncaught error occurred while running the script")
)

// errorFromLogs scans k6 logs for signs of significant errors that should be reported as such.
func errorFromLogs(logs []byte) error {
	dec := logfmt.NewDecoder(bytes.NewReader(logs))

	keyVals := make(map[string]string, 8) // Typically will be less than 8 fields.

	for dec.ScanRecord() {
		maps.Clear(keyVals)

		for dec.ScanKeyval() {
			// dec.Key and dec.Value values are not reusable across calls of ScanRecord, but that should be fine.
			keyVals[string(dec.Key())] = string(dec.Value())
		}

		if dec.Err() != nil {
			// Ignore errors.
			continue
		}

		// Stacktrace errors are often fatal, like syntax errors or accessing properties of undefined objects.
		if keyVals["level"] == "error" && keyVals["source"] == "stacktrace" {
			return ErrStacktrace
		}

		// Uncaught exceptions and errors.
		// These do not have an identifiable attribute (https://github.com/grafana/k6/issues/3842), so we rely in
		// string matching.
		if keyVals["level"] == "error" && strings.HasPrefix(keyVals["msg"], "Uncaught (in promise)") {
			return ErrThrown
		}
	}

	return nil
}
