package accounting

import (
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/require"
)

// TestGetActiveSeriesForCheck verifies that GetActiveSeriesForCheck
// returns the data in activeSeriesByCheckType. This makes sure that the
// function is applying the correct criteria to select the entry from
// the map. It also verifies that all the entries in that map are
// covered.
func TestGetActiveSeriesForCheck(t *testing.T) {
	testcases := map[string]struct {
		input                synthetic_monitoring.Check
		expectedActiveSeries int
	}{
		"dns": {
			input: synthetic_monitoring.Check{
				Target: "127.0.0.1",
				Settings: synthetic_monitoring.CheckSettings{
					Dns: &synthetic_monitoring.DnsSettings{},
				},
			},
			expectedActiveSeries: activeSeriesByCheckType["dns"],
		},
		"dns_basic": {
			input: synthetic_monitoring.Check{
				Target:           "127.0.0.1",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Dns: &synthetic_monitoring.DnsSettings{},
				},
			},
			expectedActiveSeries: activeSeriesByCheckType["dns_basic"],
		},

		"http": {
			input: synthetic_monitoring.Check{
				Target: "http://127.0.0.1/",
				Settings: synthetic_monitoring.CheckSettings{
					Http: &synthetic_monitoring.HttpSettings{},
				},
			},
			expectedActiveSeries: activeSeriesByCheckType["http"],
		},
		"http_ssl": {
			input: synthetic_monitoring.Check{
				Target: "https://127.0.0.1/",
				Settings: synthetic_monitoring.CheckSettings{
					Http: &synthetic_monitoring.HttpSettings{},
				},
			},
			expectedActiveSeries: activeSeriesByCheckType["http_ssl"],
		},
		"http_basic": {
			input: synthetic_monitoring.Check{
				Target:           "http://127.0.0.1/",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Http: &synthetic_monitoring.HttpSettings{},
				},
			},
			expectedActiveSeries: activeSeriesByCheckType["http_basic"],
		},
		"http_ssl_basic": {
			input: synthetic_monitoring.Check{
				Target:           "https://127.0.0.1/",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Http: &synthetic_monitoring.HttpSettings{},
				},
			},
			expectedActiveSeries: activeSeriesByCheckType["http_ssl_basic"],
		},

		"ping": {
			input: synthetic_monitoring.Check{
				Target: "127.0.0.1",
				Settings: synthetic_monitoring.CheckSettings{
					Ping: &synthetic_monitoring.PingSettings{},
				},
			},
			expectedActiveSeries: activeSeriesByCheckType["ping"],
		},
		"ping_basic": {
			input: synthetic_monitoring.Check{
				Target:           "127.0.0.1",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Ping: &synthetic_monitoring.PingSettings{},
				},
			},
			expectedActiveSeries: activeSeriesByCheckType["ping_basic"],
		},

		"tcp": {
			input: synthetic_monitoring.Check{
				Target: "127.0.0.1:8080",
				Settings: synthetic_monitoring.CheckSettings{
					Tcp: &synthetic_monitoring.TcpSettings{},
				},
			},
			expectedActiveSeries: activeSeriesByCheckType["tcp"],
		},
		"tcp_ssl": {
			input: synthetic_monitoring.Check{
				Target: "127.0.0.1:8080",
				Settings: synthetic_monitoring.CheckSettings{
					Tcp: &synthetic_monitoring.TcpSettings{
						Tls: true,
					},
				},
			},
			expectedActiveSeries: activeSeriesByCheckType["tcp_ssl"],
		},
		"tcp_basic": {
			input: synthetic_monitoring.Check{
				Target:           "127.0.0.1:8080",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Tcp: &synthetic_monitoring.TcpSettings{},
				},
			},
			expectedActiveSeries: activeSeriesByCheckType["tcp_basic"],
		},
		"tcp_ssl_basic": {
			input: synthetic_monitoring.Check{
				Target:           "127.0.0.1:8080",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Tcp: &synthetic_monitoring.TcpSettings{
						Tls: true,
					},
				},
			},
			expectedActiveSeries: activeSeriesByCheckType["tcp_ssl_basic"],
		},
	}

	// For know simply expect that every element in
	// activeSeriesByCheckType has a corresponding test case of the
	// same name. This ensures that if additional checks are added,
	// or new variants are introduced, they don't go unnoticed here.
	//
	// If more test cases are added, they can use names other than
	// the keys in activeSeriesByCheckType.
	for checkType := range activeSeriesByCheckType {
		_, found := testcases[checkType]
		require.True(t, found, "every element in activeSeriesByCheckType must be tested")
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			as, err := GetActiveSeriesForCheck(tc.input)
			require.NoError(t, err)
			require.Equal(t, activeSeriesByCheckType[name], as)
		})
	}
}

// TestActiveSeriesByCheckTypeInSyncWithData checks that all the entries
// in the file system have a corresponding entry in
// activeSeriesByCheckType, to make sure that code generation runs
// whenever new check types or check variants are added.
//
// FIXME(mem): the coupling between this function and the package
// layout is a little annoying.
func TestActiveSeriesByCheckTypeInSyncWithData(t *testing.T) {
	entries, err := filepath.Glob("../../internal/scraper/testdata/*.txt")
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	for _, fn := range entries {
		casename := strings.TrimSuffix(path.Base(fn), ".txt")
		_, found := activeSeriesByCheckType[casename]
		require.Truef(t, found, "case %s (%s) not found in activeSeriesByCheckType", casename, fn)
	}
}
