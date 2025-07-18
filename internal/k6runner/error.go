package k6runner

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/go-logfmt/logfmt"
	"golang.org/x/exp/maps"
)

var (
	ErrStacktrace = errors.New("fatal error occurred while running the script")
	ErrThrown     = errors.New("uncaught error occurred while running the script")
)

const (
	// ErrorCodeNone indicates no error: The script executed successfully.
	ErrorCodeNone = ""
	// ErrorCodeFailed indicates the k6 test failed and exited with a controlled, non-zero status.
	// This typically happens due to calling fail(), breached thresholds, etc.
	ErrorCodeFailed = "failed"
	// ErrorCodeTimeout occurs when the execution context is cancelled.
	ErrorCodeTimeout = "timeout"
	// ErrorCodeKilled signals that k6 exited with an error code >=128, which means that the process was killed.
	// If it is killed due to the timeout logic, ErrorCodeTimeout is returned instead.
	ErrorCodeKilled = "killed"
	// ErrorCodeAborted signals k6 exiting with an status code we map to uncontrolled failures, such as config errors,
	// javascript exceptions, etc.
	ErrorCodeAborted = "aborted"
	// ErrorCodeUnknown reperesents a non-nil error we cannot map to any known cause.
	ErrorCodeUnknown = "unknown"
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

// isUserError returns whether we attribute this error to the user, i.e. to a combination of the k6 script contents and
// settings. This includes timeouts and exit codes returned by k6.
func isUserError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	if exitErr := (&exec.ExitError{}); errors.As(err, &exitErr) && exitErr.ExitCode() < 127 {
		// If this is an ExitError and the result code is < 127, this is a user error.
		// https://github.com/grafana/k6/blob/v0.50.0/errext/exitcodes/codes.go
		return true
	}

	return false
}

// errorType returns a string representation of the error, which can be serialized and sent downstream to be later
// interpreted by the proxy and other consumers.
// TODO: This conceptually shares some logic with isUserError, we should probably refactor this at some point.
func errorType(err error) string {
	if err == nil {
		return ErrorCodeNone
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorCodeTimeout
	}

	if errors.Is(err, ErrStacktrace) {
		// Syntax error, or accessing properties of undefined variables.
		return ErrorCodeAborted
	}

	if errors.Is(err, ErrThrown) {
		// Script threw an exception.
		return ErrorCodeFailed
	}

	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		// If at this point it is not an ExitError, we don't know what happened here.
		return ErrorCodeUnknown
	}

	if exitErr.ExitCode() >= 128 {
		// As per POSIX spec, return code of a process when exiting due to an unhandled signal will be
		// 128 + signal number. This includes OOM scenarios, where the kernel sends SIGKILL to the offending process.
		// https://en.wikipedia.org/wiki/Exit_status#POSIX
		return ErrorCodeKilled
	}

	return ErrorCodeAborted
}
