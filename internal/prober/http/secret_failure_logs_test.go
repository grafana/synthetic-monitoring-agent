package http

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	kitlog "github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// TestSecretFailureReachesTheCheckLogs asserts what the owner of a check can
// read when one of its secret references does not resolve.
//
// The agent writes to two streams. The logger handed to NewProber is the
// agent's own and stays on the probe. The logger handed to Probe is the one the
// scraper collects and ships to the tenant, so it is the only one a check owner
// ever sees. Resolution failures went to the first one alone, which left the
// owner a bare "Check failed" for the one kind of failure they can fix
// themselves - less than they get told about a refused connection.
//
// Each case therefore asserts on three things: that the check owner is told
// which field and which secret, that the operator still has the failure on the
// agent's logger, and that no resolved value is in either place.
func TestSecretFailureReachesTheCheckLogs(t *testing.T) {
	secretStore := testhelper.NewMockSecretProvider(map[string]string{
		wireTokenName:    wireTokenValue,
		wirePasswordName: wirePasswordValue,
		"ca-cert":        "ca-cert-material",
		"client-key":     "client-key-material",
	})

	const missingRef = "${secrets.does-not-exist}"

	testcases := map[string]struct {
		settings sm.HttpSettings
		// wantCheckLog is asserted against the stream the tenant receives.
		wantCheckLog []string
		// wantCheckLogAbsent is asserted against the same stream.
		wantCheckLogAbsent []string
		wantSuccess        bool
	}{
		"unknown secret in bearer token": {
			settings:     sm.HttpSettings{BearerToken: missingRef},
			wantCheckLog: []string{"Could not resolve a secret referenced by this check", "bearer token", "does-not-exist"},
		},

		"unknown secret in basic auth password": {
			settings: sm.HttpSettings{
				BasicAuth: &sm.BasicAuth{Username: wireUsername, Password: missingRef},
			},
			wantCheckLog: []string{"basic auth password", "does-not-exist"},
		},

		"unknown secret in CA cert": {
			settings: sm.HttpSettings{
				TlsConfig: &sm.TLSConfig{CACert: []byte(missingRef)},
			},
			wantCheckLog: []string{"CA cert", "does-not-exist"},
		},

		"unknown secret in client key": {
			settings: sm.HttpSettings{
				TlsConfig: &sm.TLSConfig{ClientKey: []byte(missingRef)},
			},
			wantCheckLog: []string{"client key", "does-not-exist"},
		},

		// A name the pattern accepts but validation rejects is the likeliest of
		// these to be a typo, so the message has to carry the rule and not only
		// the verdict.
		"invalid secret name": {
			settings:     sm.HttpSettings{BearerToken: "${secrets.My_Token}"},
			wantCheckLog: []string{"invalid secret name", "My_Token", "lowercase letters"},
		},

		// Building the config fails for reasons that have nothing to do with
		// secrets, on a proxy URL, on TLS material and on OAuth2 settings. The
		// owner can fix those too, but not by looking at a secret, so the
		// secret wording has to stay off them.
		"a failure that is not about secrets is not called one": {
			settings:           sm.HttpSettings{ProxyURL: "http://%zz"},
			wantCheckLog:       []string{"Could not build the configuration for this check", "proxy URL"},
			wantCheckLogAbsent: []string{"secret"},
		},

		// The counterpart. A check whose references resolve says nothing about
		// secrets to its owner, because there is nothing for them to fix.
		"a reference that resolves says nothing": {
			settings:    sm.HttpSettings{BearerToken: wireTokenRef},
			wantSuccess: true,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := testhelper.Context(context.Background(), t)
			t.Cleanup(cancel)

			target := newRequestRecorder(t)
			settings := tc.settings

			check := model.Check{
				Check: sm.Check{
					Id:               1,
					TenantId:         1,
					Frequency:        10000,
					Timeout:          5000,
					Enabled:          true,
					Probes:           []int64{1},
					Target:           target.URL,
					Job:              "test",
					BasicMetricsOnly: true,
					Settings:         sm.CheckSettings{Http: &settings},
				},
			}

			var agentLog testhelper.LogBuffer
			// The scraper builds this logger over a buffer and ships what lands
			// in it, so a logfmt logger over a buffer is what the tenant sees.
			var checkLog bytes.Buffer

			prober, err := NewProber(ctx, check, zerolog.New(&agentLog), http.Header{}, secretStore)
			require.NoError(t, err)

			success, _ := prober.Probe(
				ctx,
				check.Target,
				prometheus.NewPedanticRegistry(),
				kitlog.NewLogfmtLogger(&checkLog),
				"test-execution-id",
			)

			require.Equal(t, tc.wantSuccess, success)

			// Neither stream may carry a resolved value. The buffer is never
			// empty here - a failure writes the new line and a success writes
			// the probe's own logs - so these cannot pass by default.
			require.NotEmpty(t, checkLog.String())
			require.NotContains(t, checkLog.String(), wireTokenValue)
			require.NotContains(t, checkLog.String(), wirePasswordValue)
			require.NotContains(t, agentLog.String(), wireTokenValue)

			if tc.wantSuccess {
				require.NotContains(t, checkLog.String(), "Could not resolve a secret")

				return
			}

			for _, want := range tc.wantCheckLog {
				require.Contains(t, checkLog.String(), want)
			}

			for _, absent := range tc.wantCheckLogAbsent {
				require.NotContains(t, checkLog.String(), absent)
			}

			// The operator keeps what they had before this change.
			require.Contains(t, agentLog.String(), "failed to build config for HTTP probe")
		})
	}
}
