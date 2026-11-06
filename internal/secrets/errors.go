package secrets

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

// Reasons for a secret failure. Their text is fixed, so it is safe to show the check owner.
var (
	ErrNoSecretStore           = errors.New("no secret store is configured for this stack")
	ErrSecretNotFound          = errors.New("the secret does not exist")
	ErrSecretStoreUnavailable  = errors.New("the secret store is unavailable")
	ErrSecretStoreUnauthorized = errors.New("the secret store denied the agent access")
	ErrSecretStoreBadResponse  = errors.New("the secret store returned an unexpected response")
	ErrCheckTimedOut           = errors.New("the check timed out before the secret could be fetched")
	ErrCheckCanceled           = errors.New("the check was cancelled before the secret could be fetched")
	ErrSecretFetchFailed       = errors.New("the secret could not be fetched")
)

// Error is a failure to fetch a secret or the secret store configuration. Field, Secret and
// Reason are safe for the check logs. Err is for the agent log only, because it can carry the
// secret store URL or the SM API transport detail.
type Error struct {
	// Field is the check setting that holds the reference.
	Field string
	// Secret is the secret name, left empty when it must not reach the check logs.
	Secret string
	// Reason is one of the Err* reasons above.
	Reason error
	// Err is the underlying error, if any.
	Err error
}

func (e *Error) Error() string {
	var b strings.Builder

	if e.Field != "" {
		b.WriteString("failed to resolve " + e.Field + ": ")
	}

	if e.Secret != "" {
		b.WriteString("secret '" + e.Secret + "': ")
	}

	b.WriteString(e.reason().Error())

	if e.Err != nil {
		b.WriteString(": " + e.Err.Error())
	}

	return b.String()
}

// Unwrap returns the reason and the underlying error, so errors.Is matches either.
func (e *Error) Unwrap() []error {
	if e.Err == nil {
		return []error{e.reason()}
	}

	return []error{e.reason(), e.Err}
}

// LogFields returns the field, secret and reason as key-value pairs for the check logs, leaving
// out the ones that are empty. Err is never included.
func (e *Error) LogFields() []any {
	var fields []any

	if e.Field != "" {
		fields = append(fields, "field", e.Field)
	}

	if e.Secret != "" {
		fields = append(fields, "secret", e.Secret)
	}

	return append(fields, "reason", e.reason().Error())
}

func (e *Error) reason() error {
	if e.Reason == nil {
		return ErrSecretFetchFailed
	}

	return e.Reason
}

// AsError returns a copy of the *Error in err's chain, which the caller can fill in further. Any
// other error becomes the Err of a new *Error with ErrSecretFetchFailed as its reason.
func AsError(err error) *Error {
	if secretErr, ok := errors.AsType[*Error](err); ok {
		c := *secretErr

		return &c
	}

	return &Error{Reason: ErrSecretFetchFailed, Err: err}
}

// contextReason reports why ctx ended, or nil while it is still live. It looks at ctx and not at
// the error from the call, because gRPC status errors do not wrap the context error.
func contextReason(ctx context.Context) error {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return ErrCheckTimedOut
	case ctx.Err() != nil:
		return ErrCheckCanceled
	default:
		return nil
	}
}

// classifyTransportError wraps an error from a secret store request that got no usable response.
func classifyTransportError(ctx context.Context, err error) *Error {
	reason := contextReason(ctx)
	if reason == nil {
		// The HTTP client reports almost every transport failure as a *url.Error. Anything else
		// came from decoding the response.
		if _, ok := errors.AsType[*url.Error](err); ok {
			reason = ErrSecretStoreUnavailable
		} else {
			reason = ErrSecretStoreBadResponse
		}
	}

	return &Error{Reason: reason, Err: err}
}
