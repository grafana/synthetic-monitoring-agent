package adhoc

import (
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
	waitPublished(t, publishCh)

	require.Equal(t, "real-probers", result.ExecutionId)
	require.Equal(t, int64(7), result.ProbeId)

	return result
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
		path    string
		timeout int64
		want    sm.AdHocResult_Status
	}{
		"2xx passes":            {"/ok", 5000, sm.AdHocResult_OK},
		"5xx fails":             {"/error", 5000, sm.AdHocResult_FAILED},
		"slow server times out": {"/slow", 500, sm.AdHocResult_TIMEOUT},
	} {
		t.Run(name, func(t *testing.T) {
			result := runRealCheck(t, sm.CheckSettings{Http: &sm.HttpSettings{}}, server.URL+tc.path, tc.timeout)

			require.Equal(t, tc.want, result.Status, "error: %q", result.Error)
			require.Greater(t, result.DurationSeconds, float64(0))
		})
	}
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

	// An address nothing listens on: bind a port, then release it.
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	refused := closed.Addr().String()

	require.NoError(t, closed.Close())

	for name, tc := range map[string]struct {
		target string
		want   sm.AdHocResult_Status
	}{
		"open port passes":         {open, sm.AdHocResult_OK},
		"refused connection fails": {refused, sm.AdHocResult_FAILED},
	} {
		t.Run(name, func(t *testing.T) {
			result := runRealCheck(t, sm.CheckSettings{Tcp: &sm.TcpSettings{}}, tc.target, 5000)

			require.Equal(t, tc.want, result.Status, "error: %q", result.Error)
		})
	}
}
