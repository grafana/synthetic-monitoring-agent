package k6

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestNewProber(t *testing.T) {
	ctx, cancel := testContext(t)
	t.Cleanup(cancel)

	logger := zerolog.New(zerolog.NewTestWriter(t))

	testcases := map[string]struct {
		check         synthetic_monitoring.Check
		expectFailure bool
	}{
		"valid": {
			expectFailure: false,
			check: synthetic_monitoring.Check{
				Target:    "http://www.example.org",
				Job:       "test",
				Frequency: 10 * 1000,
				Timeout:   10 * 1000,
				Probes:    []int64{1},
				Settings: synthetic_monitoring.CheckSettings{
					K6: &synthetic_monitoring.K6Settings{
						Script: []byte("// test"),
					},
				},
			},
		},
		"invalid": {
			expectFailure: true,
			check: synthetic_monitoring.Check{
				Target:    "http://www.example.org",
				Job:       "test",
				Frequency: 10 * 1000,
				Timeout:   10 * 1000,
				Probes:    []int64{1},
				Settings:  synthetic_monitoring.CheckSettings{},
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var runner noopRunner
			p, err := NewProber(ctx, tc.check, logger, runner)
			if tc.expectFailure {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, proberName, p.config.Prober)
			require.Equal(t, 10*time.Second, p.config.Timeout)
			require.Equal(t, tc.check.Settings.K6.Script, p.config.Script)
		})
	}
}

type noopRunner struct{}

func (noopRunner) WithLogger(logger *zerolog.Logger) k6runner.Runner {
	var r noopRunner
	return r
}

func (noopRunner) Run(ctx context.Context, script []byte) (*k6runner.RunResponse, error) {
	return &k6runner.RunResponse{}, nil
}

func testContext(t *testing.T) (context.Context, func()) {
	if deadline, ok := t.Deadline(); ok {
		return context.WithDeadline(context.Background(), deadline)
	}

	return context.WithTimeout(context.Background(), 10*time.Second)
}
