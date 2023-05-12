package multihttp

import (
	"context"
	"testing"
	"time"

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
					Multihttp: &synthetic_monitoring.MultiHttpSettings{
						Entries: []*synthetic_monitoring.MultiHttpEntry{
							{
								Request: &synthetic_monitoring.MultiHttpEntryRequest{
									Url: "http://www.example.org",
								},
							},
						},
					},
				},
			},
		},
		"settings must be valid": {
			expectFailure: true,
			check: synthetic_monitoring.Check{
				Target:    "http://www.example.org",
				Job:       "test",
				Frequency: 10 * 1000,
				Timeout:   10 * 1000,
				Probes:    []int64{1},
				Settings: synthetic_monitoring.CheckSettings{
					Multihttp: &synthetic_monitoring.MultiHttpSettings{
						Entries: []*synthetic_monitoring.MultiHttpEntry{
							// This is invalid because the requsest does not have a URL
							{},
						},
					},
				},
			},
		},
		"must contain multihttp settings": {
			expectFailure: true,
			check: synthetic_monitoring.Check{
				Target:    "http://www.example.org",
				Job:       "test",
				Frequency: 10 * 1000,
				Timeout:   10 * 1000,
				Probes:    []int64{1},
				Settings: synthetic_monitoring.CheckSettings{
					// The settings are valid for ping, but not for multihttp
					Ping: &synthetic_monitoring.PingSettings{},
				},
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			p, err := NewProber(ctx, tc.check, logger)
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

func testContext(t *testing.T) (context.Context, func()) {
	if deadline, ok := t.Deadline(); ok {
		return context.WithDeadline(context.Background(), deadline)
	}

	return context.WithTimeout(context.Background(), 10*time.Second)
}
