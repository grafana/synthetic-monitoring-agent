package scripted

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestNewProber(t *testing.T) {
	ctx, cancel := testContext(t)
	t.Cleanup(cancel)

	logger := zerolog.New(zerolog.NewTestWriter(t))

	testcases := map[string]struct {
		check         model.Check
		expectFailure bool
	}{
		"valid": {
			expectFailure: false,
			check: model.Check{
				Check: sm.Check{
					Target:    "http://www.example.org",
					Job:       "test",
					Frequency: 10 * 1000,
					Timeout:   10 * 1000,
					Probes:    []int64{1},
					Settings: sm.CheckSettings{
						Scripted: &sm.ScriptedSettings{
							Script: []byte("// test"),
						},
					},
				},
			},
		},
		"invalid": {
			expectFailure: true,
			check: model.Check{
				Check: sm.Check{
					Target:    "http://www.example.org",
					Job:       "test",
					Frequency: 10 * 1000,
					Timeout:   10 * 1000,
					Probes:    []int64{1},
					Settings:  sm.CheckSettings{},
				},
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var runner noopRunner
			var store testhelper.NoopSecretStore
			p, err := NewProber(ctx, tc.check, logger, runner, store)
			if tc.expectFailure {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, proberName, p.module.Prober)
			require.Equal(t, 10*time.Second, time.Duration(p.module.Script.Settings.Timeout)*time.Millisecond)
			require.Equal(t, tc.check.Settings.Scripted.Script, p.module.Script.Script)
		})
	}
}

type noopRunner struct{}

func (noopRunner) WithLogger(logger *zerolog.Logger) k6runner.Runner {
	var r noopRunner
	return r
}

func (noopRunner) Run(ctx context.Context, script k6runner.Script, secretStore k6runner.SecretStore) (*k6runner.RunResponse, error) {
	return &k6runner.RunResponse{}, nil
}

func testContext(t *testing.T) (context.Context, func()) {
	if deadline, ok := t.Deadline(); ok {
		return context.WithDeadline(context.Background(), deadline)
	}

	return context.WithTimeout(context.Background(), 10*time.Second)
}
