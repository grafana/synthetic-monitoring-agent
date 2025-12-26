package multihttp

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
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
	ctx, cancel := testhelper.Context(context.Background(), t)
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
		},
		"settings must be valid": {
			expectFailure: true,
			check: model.Check{
				Check: sm.Check{
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
		},
		"must contain multihttp settings": {
			expectFailure: true,
			check: model.Check{
				Check: sm.Check{
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
		},
		"header overwrite protection is case-insensitive": {
			expectFailure: false,
			check: model.Check{
				Check: sm.Check{
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
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var runner noopRunner
			var store testhelper.NoopSecretStore
			checkId := tc.check.Id
			reservedHeaders := http.Header{}
			reservedHeaders.Add("x-sm-id", fmt.Sprintf("%d-%d", checkId, checkId))

			p, err := NewProber(ctx, tc.check, logger, runner, reservedHeaders, store)
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

func (noopRunner) Run(ctx context.Context, script k6runner.Script, secretStore k6runner.SecretStore) (*k6runner.RunResponse, error) {
	return &k6runner.RunResponse{}, nil
}

func TestAugmentHTTPHeaders(t *testing.T) {
	t.Parallel()

	testReservedHeaders := http.Header{}
	testReservedHeaders.Add("reserved", "value")
	testReservedHeaders.Add("also-reserved", "another-value")

	for _, tc := range []struct {
		name          string
		entries       []*sm.MultiHttpEntry
		expectHeaders [][]*sm.HttpHeader
	}{
		{
			name: "reserved headers are added to all requests",
			entries: []*sm.MultiHttpEntry{
				{
					Request: &sm.MultiHttpEntryRequest{
						Headers: []*sm.HttpHeader{},
					},
				},
				{
					Request: &sm.MultiHttpEntryRequest{
						Headers: []*sm.HttpHeader{},
					},
				},
			},
			expectHeaders: [][]*sm.HttpHeader{
				{
					{Name: "Reserved", Value: "value"},
					{Name: "Also-Reserved", Value: "another-value"},
				},
				{
					{Name: "Reserved", Value: "value"},
					{Name: "Also-Reserved", Value: "another-value"},
				},
			},
		},
		{
			name: "reserved headers are not overridable",
			entries: []*sm.MultiHttpEntry{
				{
					Request: &sm.MultiHttpEntryRequest{
						Headers: []*sm.HttpHeader{
							{Name: "reserved", Value: "I should not be here"},
						},
					},
				},
			},
			expectHeaders: [][]*sm.HttpHeader{
				{
					{Name: "Reserved", Value: "value"},
					{Name: "Also-Reserved", Value: "another-value"},
				},
			},
		},
		{
			name: "headers for different requests are preserved",
			entries: []*sm.MultiHttpEntry{
				{
					Request: &sm.MultiHttpEntryRequest{
						Headers: []*sm.HttpHeader{
							{Name: "request", Value: "1"},
							{Name: "request-1", Value: "1"},
						},
					},
				},
				{
					Request: &sm.MultiHttpEntryRequest{
						Headers: []*sm.HttpHeader{
							{Name: "request", Value: "2"},
							{Name: "request-2", Value: "2"},
						},
					},
				},
				{
					Request: &sm.MultiHttpEntryRequest{
						Headers: []*sm.HttpHeader{
							{Name: "request", Value: "3"},
							{Name: "request-3", Value: "3"},
						},
					},
				},
			},
			expectHeaders: [][]*sm.HttpHeader{
				{
					{Name: "Reserved", Value: "value"},
					{Name: "Also-Reserved", Value: "another-value"},
					{Name: "request", Value: "1"},
					{Name: "request-1", Value: "1"},
				},
				{
					{Name: "Reserved", Value: "value"},
					{Name: "Also-Reserved", Value: "another-value"},
					{Name: "request", Value: "2"},
					{Name: "request-2", Value: "2"},
				},
				{
					{Name: "Reserved", Value: "value"},
					{Name: "Also-Reserved", Value: "another-value"},
					{Name: "request", Value: "3"},
					{Name: "request-3", Value: "3"},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			check := sm.Check{
				Id: 0,
				Settings: sm.CheckSettings{
					Multihttp: &sm.MultiHttpSettings{
						Entries: tc.entries,
					},
				},
			}

			augmentHttpHeaders(&check, testReservedHeaders)

			require.Equalf(t, len(check.Settings.Multihttp.Entries), len(tc.entries), "Unexpected number of entries")

			for i := range check.Settings.Multihttp.Entries {
				// Sort header slices for easier comparison.
				actual := check.Settings.Multihttp.Entries[i].Request.Headers
				slices.SortFunc(actual, func(a, b *sm.HttpHeader) int {
					return strings.Compare(a.Name, b.Name)
				})

				expected := tc.expectHeaders[i]
				slices.SortFunc(expected, func(a, b *sm.HttpHeader) int {
					return strings.Compare(a.Name, b.Name)
				})

				require.Equalf(t, expected, actual, "Actual headers do not match expected")
			}
		})
	}
}
