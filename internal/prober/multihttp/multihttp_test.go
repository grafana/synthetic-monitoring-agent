package multihttp

import (
	"context"
	"fmt"
	"net/http"
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
				Id:        1,
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
				Id:        2,
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
				Id:        3,
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
		"header overwrite protection is case-insensitive": {
			expectFailure: false,
			check: sm.Check{
				Id:        4,
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
									Url:     "http://www.example.org",
									Headers: []*sm.HttpHeader{{Name: "X-sM-Id", Value: "9880-9880"}},
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var runner noopRunner
			checkId := tc.check.Id
			reservedHeaders := http.Header{}
			reservedHeaders.Add("x-sm-id", fmt.Sprintf("%d-%d", checkId, checkId))

			p, err := NewProber(ctx, tc.check, logger, runner, reservedHeaders)
			if tc.expectFailure {
				require.Error(t, err)
				return
			}

			requestHeaders := tc.check.Settings.Multihttp.Entries[0].Request.Headers
			require.Equal(t, len(requestHeaders), 1) // reserved header is present

			require.Equal(t, requestHeaders[0].Name, "X-Sm-Id")
			require.Equal(t, requestHeaders[0].Value, fmt.Sprintf("%d-%d", checkId, checkId))

			require.NoError(t, err)
			require.Equal(t, proberName, p.module.Prober)
			require.Equal(t, 10*time.Second, time.Duration(p.module.Script.Settings.Timeout)*time.Millisecond)
			// TODO: check script
		})
	}
}

type noopRunner struct{}

func (noopRunner) WithLogger(logger *zerolog.Logger) k6runner.Runner {
	var r noopRunner
	return r
}

func (noopRunner) Run(ctx context.Context, script k6runner.Script) (*k6runner.RunResponse, error) {
	return &k6runner.RunResponse{}, nil
}
