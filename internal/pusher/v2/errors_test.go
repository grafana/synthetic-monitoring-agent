package v2

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/prom"
)

func TestParsePushError(t *testing.T) {
	for title, tc := range map[string]struct {
		input    error
		expected error
		code     int
	}{
		"no error": {
			input: nil,
			expected: pushError{
				kind: errKindNoError,
			},
			code: http.StatusOK,
		},

		"context canceled": {
			input: context.Canceled,
			expected: pushError{
				kind: errKindTerminated,
			},
			code: 0,
		},

		"context deadline exceeded": {
			input: context.DeadlineExceeded,
			expected: pushError{
				kind: errKindNetwork,
			},
			code: 0,
		},

		"other error is network": {
			input: errors.New("something is wrong"),
			expected: pushError{
				kind: errKindNetwork,
			},
			code: 0,
		},

		"500 errors are temporary": {
			input: &prom.HttpError{
				StatusCode: 500,
				Status:     "Internal Server Error",
				Err:        errors.New(`something is wrong`),
			},
			expected: pushError{
				kind: errKindNetwork,
			},
			code: 500,
		},

		"500 from grafana instance": {
			input: &prom.HttpError{
				StatusCode: 500,
				Status:     "Internal Server Error",
				Err:        errors.New(`It looks like there is an issue with this instance`),
			},
			expected: pushError{
				kind: errKindTenant,
			},
			code: 500,
		},

		"400 errors are data errors": {
			input: &prom.HttpError{
				StatusCode: 400,
				Status:     "Bad Request",
				Err:        errors.New(`400 Bad Request: couldn't parse labels: 1:248: parse error: unexpected identifier "foo" in label set, expected "," or "}"`),
			},
			expected: pushError{
				kind: errKindPayload,
			},
			code: 400,
		},

		"400 err-mimir-sample-out-of-order": {
			input: &prom.HttpError{
				StatusCode: 400,
				Status:     "Bad Request",
				Err:        errors.New(`400 Bad Request: failed pushing to ingester: user=275106: the sample has been rejected because another sample with a more recent timestamp has already been ingested and out-of-order samples are not allowed (err-mimir-sample-out-of-order). The affected sample has timestamp`),
			},
			expected: pushError{
				kind: errKindPayload,
			},
			code: 400,
		},

		"400 err-mimir-max-series-per-user": {
			input: &prom.HttpError{
				StatusCode: 400,
				Status:     "Bad Request",
				Err:        errors.New(`400 Bad Request: failed pushing to ingester: user=1234: per-user series limit of 15000 exceeded (err-mimir-max-series-per-user). To adjust the related per-tenant limit, configure -ingester.max-global-series-per-user, or contact your service administrator.`),
			},
			expected: pushError{
				kind: errKindFatal,
			},
			code: 400,
		},

		"429 generic": {
			input: &prom.HttpError{
				StatusCode: 429,
				Status:     "Too Many Requests",
				Err:        errors.New(`the request has been rejected because the tenant exceeded the request rate limit, set to 75 requests/s across all distributors with a maximum allowed burst of 750 (err-mimir-tenant-max-request-rate)`),
			},
			expected: pushError{
				kind: errKindWait,
			},
			code: 429,
		},

		"429 loki ingest denied": {
			input: &prom.HttpError{
				StatusCode: 429,
				Status:     "Too Many Requests",
				Err:        errors.New(`Ingestion rate limit exceeded for user 1234 (limit: 0 bytes/sec) while attempting to ingest '7' lines totaling '1874' bytes, reduce log volume or contact your Loki administrator to see if the limit can be increased`),
			},
			expected: pushError{
				kind: errKindFatal,
			},
			code: 429,
		},

		"429 loki active streams": {
			input: &prom.HttpError{
				StatusCode: 429,
				Status:     "Too Many Requests",
				Err:        errors.New(`Maximum active stream limit exceeded, reduce the number of active streams (reduce labels or reduce label values), or contact your Loki administrator to see if the limit can be increased, user: '1234'`),
			},
			expected: pushError{
				kind: errKindFatal,
			},
			code: 429,
		},

		"404 Not Found": {
			input: &prom.HttpError{
				StatusCode: 404,
				Status:     "Not Found",
				Err:        errors.New("file not found"),
			},
			expected: pushError{
				kind: errKindFatal,
			},
			code: 404,
		},

		"401 bad token": {
			input: &prom.HttpError{
				StatusCode: 401,
				Status:     "Unauthorized",
				Err:        errors.New(`{"status":"error","error":"authentication error: invalid token"}`),
			},
			expected: pushError{
				kind: errKindTenant,
			},
			code: 401,
		},

		"403 Forbidden": {
			input: &prom.HttpError{
				StatusCode: 403,
				Status:     "Forbidden",
				Err:        errors.New(`Forbidden`),
			},
			expected: pushError{
				kind: errKindFatal,
			},
			code: 403,
		},

		"300 response": {
			input: &prom.HttpError{
				StatusCode: 300,
				Status:     "Multiple Choices",
				Err:        errors.New("multiple choices"),
			},
			expected: pushError{
				kind: errKindFatal,
			},
			code: 300,
		},
	} {
		t.Run(title, func(t *testing.T) {
			code, err := parsePublishError(tc.input)
			var (
				expectedErr = tc.expected
				pErr        pushError
			)
			if errors.As(expectedErr, &pErr) {
				pErr.inner = tc.input
				expectedErr = pErr
			}
			require.Equal(t, expectedErr, err)
			require.Equal(t, tc.code, code)
		})
	}
}
