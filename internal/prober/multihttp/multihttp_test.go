package multihttp

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestNewProber(t *testing.T) {
	ctx, cancel := testhelper.Context(context.Background(), t)
	t.Cleanup(cancel)

	logger := zerolog.New(zerolog.NewTestWriter(t))

	testcases := map[string]struct {
		check         sm.Check
		expectFailure bool
	}{
		"valid": {
			expectFailure: false,
			check: sm.Check{
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
									QueryFields: []*sm.QueryField{
										{
											Name:  "q",
											Value: "${v}",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"settings must be valid": {
			expectFailure: true,
			check: sm.Check{
				Target:    "http://www.example.org",
				Job:       "test",
				Frequency: 10 * 1000,
				Timeout:   10 * 1000,
				Probes:    []int64{1},
				Settings: sm.CheckSettings{
					Multihttp: &sm.MultiHttpSettings{
						Entries: []*sm.MultiHttpEntry{
							// This is invalid because the requsest does not have a URL
							{},
						},
					},
				},
			},
		},
		"must contain multihttp settings": {
			expectFailure: true,
			check: sm.Check{
				Target:    "http://www.example.org",
				Job:       "test",
				Frequency: 10 * 1000,
				Timeout:   10 * 1000,
				Probes:    []int64{1},
				Settings: sm.CheckSettings{
					// The settings are valid for ping, but not for multihttp
					Ping: &sm.PingSettings{},
				},
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
			// TODO: check script
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
