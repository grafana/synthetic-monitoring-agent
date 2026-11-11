package adhoc

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// These tests run real probers, built by the default runner factory,
// against local servers, and check the status reported to the API. The
// other tests use fake probers.
//
// Only protocol checks are covered: checks that need k6 (scripted,
// multihttp and browser) are not.

// runRealCheck runs one ad-hoc check through the whole handler and
// returns the result reported to the API.
func runRealCheck(t *testing.T, settings sm.CheckSettings, target string, timeoutMs int64) *sm.AdHocResult {
	t.Helper()

	srv := newTransportServer(&sm.AdHocRequest{
		AdHocCheck: sm.AdHocCheck{
			Id:       "real-probers",
			TenantId: 1,
			Target:   target,
			Timeout:  timeoutMs,
			Settings: settings,
		},
	})

	// A nil factory selects the default one, which builds real probers.
	publishCh := startTransportHandler(t, srv, nil)

	result := srv.waitReported(t)
	payload := waitPublished(t, publishCh)

	// Handy to see what the prober logged when a test fails.
	t.Logf("loki line: %s", payload.Streams()[0].Entries[0].Line)

	require.Equal(t, "real-probers", result.ExecutionId)
	require.Equal(t, int64(7), result.ProbeId)

	return result
}

// closedAddress returns an address, on this machine, where nothing is
// listening.
func closedAddress(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := l.Addr().String()

	require.NoError(t, l.Close())

	return addr
}

func TestRealProbersHTTP(t *testing.T) {
	// Handlers that must be released before the server can close.
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)

		case "/error":
			w.WriteHeader(http.StatusInternalServerError)

		case "/slow":
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}
	}))

	// Cleanups run last in, first out: release the handlers, then close.
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	for name, tc := range map[string]struct {
		target  string
		timeout int64
		want    sm.AdHocResult_Status
		// wantError lists what the reason for a failure must contain.
		wantError []string
	}{
		"2xx passes":            {server.URL + "/ok", 5000, sm.AdHocResult_OK, nil},
		"5xx fails":             {server.URL + "/error", 5000, sm.AdHocResult_FAILED, []string{"Invalid HTTP response status code", "500"}},
		"slow server times out": {server.URL + "/slow", 500, sm.AdHocResult_TIMEOUT, []string{"Error for HTTP request", "context deadline exceeded"}},
		"refused connection":    {"http://" + closedAddress(t) + "/", 5000, sm.AdHocResult_FAILED, []string{"Error for HTTP request", "connection refused"}},
	} {
		t.Run(name, func(t *testing.T) {
			result := runRealCheck(t, sm.CheckSettings{Http: &sm.HttpSettings{}}, tc.target, tc.timeout)

			require.Equal(t, tc.want, result.Status, "error: %q", result.Error)
			require.Greater(t, result.DurationSeconds, float64(0))

			if tc.wantError == nil {
				require.Empty(t, result.Error, "a passing check has no failure reason")
			}

			for _, s := range tc.wantError {
				require.Contains(t, result.Error, s)
			}

			require.LessOrEqual(t, len(result.Error), maxReasonBytes)
		})
	}
}

// The reason a check failed goes to the API, so it must not carry
// credentials the check was configured with.
func TestRealProbersFailureReasonHasNoCredentials(t *testing.T) {
	const password = "s3cretpassw0rd"

	target := fmt.Sprintf("http://user:%s@%s/", password, closedAddress(t))

	result := runRealCheck(t, sm.CheckSettings{Http: &sm.HttpSettings{}}, target, 5000)

	t.Logf("reason: %s", result.Error)

	require.Equal(t, sm.AdHocResult_FAILED, result.Status)
	require.Contains(t, result.Error, "connection refused", "there must be a reason for this to mean anything")
	require.NotContains(t, result.Error, password)
}

func TestRealProbersTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	open := listener.Addr().String()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			_ = conn.Close()
		}
	}()

	t.Cleanup(func() { _ = listener.Close() })

	for name, tc := range map[string]struct {
		target    string
		want      sm.AdHocResult_Status
		wantError []string
	}{
		"open port passes":         {open, sm.AdHocResult_OK, nil},
		"refused connection fails": {closedAddress(t), sm.AdHocResult_FAILED, []string{"Error dialing TCP", "connection refused"}},
	} {
		t.Run(name, func(t *testing.T) {
			result := runRealCheck(t, sm.CheckSettings{Tcp: &sm.TcpSettings{}}, tc.target, 5000)

			require.Equal(t, tc.want, result.Status, "error: %q", result.Error)

			if tc.wantError == nil {
				require.Empty(t, result.Error, "a passing check has no failure reason")
			}

			for _, s := range tc.wantError {
				require.Contains(t, result.Error, s)
			}
		})
	}
}
