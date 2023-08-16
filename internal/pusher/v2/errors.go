package v2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/prom"
)

// errKind enum used to carry the category of a push error.
type errKind int8

const (
	errKindNoError    errKind = iota // No error (never returned, always nil error).
	errKindNetwork                   // Transient network error or other retriable error
	errKindPayload                   // There is a problem with the data being sent. Discard it.
	errKindWait                      // Sending too much data, delay publishing
	errKindTenant                    // A problem with the tenant remotes. Fetch the tenant again.
	errKindFatal                     // There is a problem that can't be fixed by fetching the tenant.
	errKindTerminated                // Push terminated (context canceled)
)

func (k errKind) String() string {
	switch k {
	case errKindNoError:
		return "no error"
	case errKindNetwork:
		return "network error"
	case errKindPayload:
		return "payload error"
	case errKindWait:
		return "waitable error"
	case errKindTenant:
		return "tenant error"
	case errKindFatal:
		return "fatal error"
	case errKindTerminated:
		return "terminate error"
	}
	return "unknown error"
}

// pushError encapsulates an existing error with an errKind
type pushError struct {
	kind  errKind
	inner error
}

func (e pushError) Error() string {
	return fmt.Sprintf("%s: %s", e.kind.String(), e.inner)
}

func (e pushError) Unwrap() error {
	return e.inner
}

func (e pushError) Kind() errKind {
	return e.kind
}

func (e pushError) IsRetriable() bool {
	return e.kind == errKindNetwork
}

type alternativeMapping struct {
	substr string
	kind   errKind
}

// httpCodeMappings maps an HTTP Response Code to an errKind.
// Alternative mappings can be provided by inspecting the error returned by the server.
var httpCodeMappings = map[int]struct {
	kind         errKind
	alternatives []alternativeMapping
}{
	1: { // 1xx: retriable error
		kind: errKindNetwork,
	},
	2: { // 2xx: No error
		kind: errKindNoError,
	},
	3: { // 3xx: These usually indicate a misconfiguration (tenant is pointing to the wrong url?)
		kind: errKindFatal,
	},
	4: { // 4xx: Not an error. Just part of the payload is unacceptable.
		kind: errKindPayload,
	},
	5: { // 5xx: Transient error.
		kind: errKindNetwork,
	},

	http.StatusInternalServerError: { // 500
		kind: errKindNetwork,
		alternatives: []alternativeMapping{
			{
				substr: "looks like there is an issue with this instance",
				kind:   errKindTenant,
			},
		},
	},

	http.StatusBadRequest: { // 400
		kind: errKindPayload,
		alternatives: []alternativeMapping{
			{
				substr: "err-mimir-max-series-per-user",
				kind:   errKindFatal,
			},
		},
	},

	http.StatusTooManyRequests: { // 429
		kind: errKindWait,
		alternatives: []alternativeMapping{
			{
				substr: "limit: 0 ",
				kind:   errKindFatal,
			},
			{
				substr: "Maximum active stream limit exceeded",
				kind:   errKindFatal,
			},
		},
	},

	// Specific 4xx messages that don't translate to data error

	http.StatusUnauthorized: { // 401
		kind: errKindTenant,
	},

	http.StatusForbidden: { // 403
		kind: errKindFatal,
	},

	http.StatusNotFound: { // 404
		kind: errKindFatal,
	},

	http.StatusMethodNotAllowed: { // 405
		kind: errKindFatal,
	},

	http.StatusRequestTimeout: { // 408
		kind: errKindNetwork,
	},
}

// parsePublishError parses the error resulting from a publish operation and converts it into a pushError.
// The only exception is any error that wraps a context.Canceled error. In that case, context.Canceled
// is returned.
func parsePublishError(err error) (httpStatusCode int, pushErr pushError) {
	const noHTTPCode = 0

	if err == nil {
		return http.StatusOK, pushError{
			kind:  errKindNoError,
			inner: nil,
		}
	}

	// Context errors can be wrapped by various other error types, like
	// prom.recoverableError and url.Error.
	if errors.Is(err, context.Canceled) {
		return noHTTPCode, pushError{
			kind:  errKindTerminated,
			inner: context.Canceled,
		}
	}

	// Any DeadlineExceeded is assumed to be a network timeout of some kind.
	if errors.Is(err, context.DeadlineExceeded) {
		return noHTTPCode, pushError{
			kind:  errKindNetwork,
			inner: err,
		}
	}

	code, hasStatusCode := prom.GetHttpStatusCode(err)
	if !hasStatusCode {
		// Errors without an HTTP Status code are treated as network errors.
		return noHTTPCode, pushError{
			kind:  errKindNetwork,
			inner: err,
		}
	}

	mapping, found := httpCodeMappings[code]
	if !found {
		// No mapping for this specific HTTP status code. Try a general 5xx/4xx/etc.
		if mapping, found = httpCodeMappings[code/100]; !found {
			// No mapping for this http status at all?
			// This should never happen.
			return code, pushError{
				kind:  errKindFatal,
				inner: err,
			}
		}
	}

	// Check specific alternatives that look into the error message.
	errText := err.Error()
	for _, alt := range mapping.alternatives {
		if strings.Contains(errText, alt.substr) {
			return code, pushError{
				kind:  alt.kind,
				inner: err,
			}
		}
	}

	// return base mapping for this status code.
	return code, pushError{
		kind:  mapping.kind,
		inner: err,
	}
}
