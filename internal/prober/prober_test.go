package prober

import (
	"bytes"
	"context"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestProberFactoryCoverage(t *testing.T) {
	// This test will assert that the prober factory is handling all the
	// known check types (as defined in the synthetic_monitoring package).
	var store testhelper.NoopSecretStore

	pf := NewProberFactory(nil, 0, feature.Collection{}, &store)
	ctx := context.Background()
	testLogger := zerolog.New(zerolog.NewTestWriter(t))

	for _, checkType := range sm.CheckTypeValues() {
		var check model.Check
		require.NoError(t, check.FromSM(sm.GetCheckInstance(checkType)))

		_, _, err := pf.New(ctx, testLogger, check)
		require.NotErrorIs(t, err, errUnsupportedCheckType)
	}
}

// scriptCapturingRunner is a k6runner.Runner that records the script it is
// asked to run.
type scriptCapturingRunner struct {
	script *[]byte
}

func (r scriptCapturingRunner) WithLogger(*zerolog.Logger) k6runner.Runner {
	return r
}

func (r scriptCapturingRunner) Run(_ context.Context, script k6runner.Script, _ k6runner.SecretStore, _ string) (*k6runner.RunResponse, error) {
	*r.script = script.Script

	return &k6runner.RunResponse{}, nil
}

func (scriptCapturingRunner) Versions(context.Context) <-chan []string {
	return nil
}

type discardLogger struct{}

func (discardLogger) Log(...any) error { return nil }

// TestProberFactoryMultiHTTPLeanImports verifies that the
// multihttp-lean-imports feature flag reaches the MultiHTTP script generator.
func TestProberFactoryMultiHTTPLeanImports(t *testing.T) {
	// A check that needs neither jsonpath nor k6/encoding.
	check := model.Check{
		Check: sm.Check{
			Id:        1,
			TenantId:  1,
			Target:    "http://www.example.org",
			Job:       "test",
			Frequency: 10 * 1000,
			Timeout:   10 * 1000,
			Probes:    []int64{1},
			Settings: sm.CheckSettings{
				Multihttp: &sm.MultiHttpSettings{
					Entries: []*sm.MultiHttpEntry{
						{
							Request: &sm.MultiHttpEntryRequest{
								Url: "http://www.example.org",
							},
						},
					},
				},
			},
		},
	}

	testcases := map[string]struct {
		features   feature.Collection
		wantLegacy bool
	}{
		"flag unset keeps all imports": {
			features:   feature.Collection{},
			wantLegacy: true,
		},
		"flag set drops unused imports": {
			features:   feature.Collection{feature.MultiHTTPLeanImports: {}},
			wantLegacy: false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var (
				script []byte
				store  testhelper.NoopSecretStore
			)

			pf := NewProberFactory(scriptCapturingRunner{script: &script}, 0, tc.features, &store)
			logger := zerolog.New(zerolog.NewTestWriter(t))

			p, _, err := pf.New(context.Background(), logger, check)
			require.NoError(t, err)

			// The script is only handed to the runner when the prober runs.
			// The result of the run is irrelevant here.
			_, _ = p.Probe(context.Background(), check.Target, prometheus.NewRegistry(), discardLogger{}, "")

			require.NotEmpty(t, script, "the runner did not receive a script")

			// k6/execution is imported by the legacy script and never by the
			// lean one.
			require.Equal(t, tc.wantLegacy, bytes.Contains(script, []byte("k6/execution")))
			require.Equal(t, tc.wantLegacy, bytes.Contains(script, []byte("k6/encoding")))
			require.Equal(t, tc.wantLegacy, bytes.Contains(script, []byte("jslib.k6.io/jsonpath")))
		})
	}
}
