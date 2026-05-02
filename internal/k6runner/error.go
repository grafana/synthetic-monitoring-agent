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
	// ErrorCodeBrowser indicates the runner could not reach the browser session manager (crocochrome). Returned by the
	// legacy sm-k6-runner; mapped to HTTP 503.
	ErrorCodeBrowser = "browser"
	// ErrorCodeUnsupportedVersion indicates that the requested k6 channel manifest does not match any installed binary.
	// Returned by the legacy sm-k6-runner; mapped to HTTP 422.
	ErrorCodeUnsupportedVersion = "unsupported-version"
	// ErrorCodeBadVersion indicates that the requested k6 channel manifest is not a valid semver constraint.
	// Returned by the legacy sm-k6-runner; mapped to HTTP 422.
	ErrorCodeBadVersion = "bad-version"

	// ErrorCodeDispatchCapacity signals that the runner service had no worker available within the configured hold
	// interval. Retriable; clients should apply standard backoff.
	ErrorCodeDispatchCapacity = "dispatch_capacity"
	// ErrorCodeDispatcherDrain signals that the dispatcher (or a tier shard of it) is being deliberately drained.
	// In-queue requests that have not yet started executing are returned to the client with this code so they can be
	// rescheduled immediately, with no backoff and without counting as a failure.
	ErrorCodeDispatcherDrain = "dispatcher_drain"
	// ErrorCodeWorkerCrashPreScript signals that the worker died before the script started executing. The dispatcher
	// may transparently reschedule onto another worker.
	ErrorCodeWorkerCrashPreScript = "worker_crash_pre_script"
	// ErrorCodeWorkerCrashMidScript signals that the worker died after the script started executing. The check fails
	// this tick; the next scheduled tick handles recovery (no silent server-side retry of side-effect-bearing scripts).
	ErrorCodeWorkerCrashMidScript = "worker_crash_mid_script"
	// ErrorCodeSandboxIsolation signals a failure in the per-check sandbox layer (cgroup attach, namespace creation,
	// etc.). Should be near-zero; non-zero indicates broken isolation.
	ErrorCodeSandboxIsolation = "sandbox_isolation_error"
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
