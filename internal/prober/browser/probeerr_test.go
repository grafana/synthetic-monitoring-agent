package browser

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type stubRunner struct {
	resp *k6runner.RunResponse
	err  error
}

func (r stubRunner) WithLogger(*zerolog.Logger) k6runner.Runner { return r }

func (r stubRunner) Run(context.Context, k6runner.Script, k6runner.SecretStore, string) (*k6runner.RunResponse, error) {
	return r.resp, r.err
}

func (stubRunner) Versions(context.Context) <-chan []string { return nil }

type discardLogger struct{}

func (discardLogger) Log(...any) error { return nil }

// A check that could not be run is not the same as a check that failed:
// ProbeErr tells them apart, and Probe keeps reporting both as a failure.
func TestProbeErr(t *testing.T) {
	ctx, cancel := testhelper.Context(context.Background(), t)
	t.Cleanup(cancel)

	runnerErr := errors.New("runner unreachable")

	check := model.Check{
		Check: sm.Check{
			Target:    "http://www.example.org",
			Job:       "test",
			Frequency: 10 * 1000,
			Timeout:   10 * 1000,
			Probes:    []int64{1},
			Settings:  sm.CheckSettings{Browser: &sm.BrowserSettings{Script: []byte("// test")}},
		},
	}

	for name, tc := range map[string]struct {
		runner  stubRunner
		success bool
		err     error
	}{
		"the runner fails":  {stubRunner{err: runnerErr}, false, runnerErr},
		"the script fails":  {stubRunner{resp: &k6runner.RunResponse{Error: "boom", ErrorCode: "failed"}}, false, nil},
		"the script passes": {stubRunner{resp: &k6runner.RunResponse{}}, true, nil},
	} {
		t.Run(name, func(t *testing.T) {
			p, err := NewProber(ctx, check, zerolog.New(io.Discard), tc.runner, testhelper.NoopSecretStore{})
			require.NoError(t, err)

			success, _, err := p.ProbeErr(ctx, check.Target, prometheus.NewRegistry(), discardLogger{}, "id")
			require.Equal(t, tc.success, success)

			if tc.err == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.err)
			}

			// Probe still reports the same outcome, without the error.
			probeSuccess, probeDuration := p.Probe(ctx, check.Target, prometheus.NewRegistry(), discardLogger{}, "id")
			require.Equal(t, tc.success, probeSuccess)

			if tc.err != nil {
				require.Zero(t, probeDuration)
			}
		})
	}
}
