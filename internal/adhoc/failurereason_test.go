package adhoc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFailureReason(t *testing.T) {
	for name, tc := range map[string]struct {
		entries []map[string]string
		want    string
	}{
		"no entries": {
			entries: nil,
			want:    "",
		},
		"no errors": {
			entries: []map[string]string{
				{"level": "DEBUG", "msg": "Resolving target address"},
				{"level": "INFO", "msg": "something"},
			},
			want: "",
		},
		"message and error": {
			entries: []map[string]string{
				{"level": "DEBUG", "msg": "Making HTTP request"},
				{"level": "ERROR", "msg": "Error dialing TCP", "err": "connection refused"},
			},
			want: "Error dialing TCP: connection refused",
		},
		"other fields are kept, in order": {
			entries: []map[string]string{
				{"level": "ERROR", "msg": "Invalid HTTP response status code, wanted 2xx", "status_code": "500", "a": "1", "time": "now"},
			},
			want: "Invalid HTTP response status code, wanted 2xx (a=1, status_code=500)",
		},
		"the last error wins": {
			entries: []map[string]string{
				{"level": "ERROR", "msg": "first"},
				{"level": "DEBUG", "msg": "in between"},
				{"level": "ERROR", "msg": "last"},
				{"level": "DEBUG", "msg": "after"},
			},
			want: "last",
		},
		"level is not case sensitive": {
			entries: []map[string]string{{"level": "error", "msg": "lowercase"}},
			want:    "lowercase",
		},
		"go-kit style entries": {
			entries: []map[string]string{{"level": "error", "msg": "Error resolving address", "err": "no such host"}},
			want:    "Error resolving address: no such host",
		},
		"error without a message": {
			entries: []map[string]string{{"level": "ERROR", "err": "boom"}},
			want:    "boom",
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, failureReason(tc.entries))
		})
	}
}

func TestFailureReasonIsBounded(t *testing.T) {
	reason := failureReason([]map[string]string{
		{"level": "ERROR", "msg": "boom", "err": strings.Repeat("x", 10*maxReasonBytes)},
	})

	require.LessOrEqual(t, len(reason), maxReasonBytes)
	require.True(t, strings.HasPrefix(reason, "boom: xxx"))
}

// Truncating in the middle of a multibyte character would make the
// reason invalid UTF-8, which protobuf strings must not be.
func TestFailureReasonIsValidUTF8(t *testing.T) {
	reason := failureReason([]map[string]string{
		{"level": "ERROR", "msg": "boom", "err": strings.Repeat("é", maxReasonBytes)},
	})

	require.LessOrEqual(t, len(reason), maxReasonBytes)
	require.True(t, strings.ToValidUTF8(reason, "") == reason, "must be valid UTF-8")
}
