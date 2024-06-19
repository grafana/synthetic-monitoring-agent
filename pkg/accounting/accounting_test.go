package accounting

import (
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/require"
)

func getTestCases() map[string]struct {
	input synthetic_monitoring.Check
	class string
} {
	return map[string]struct {
		input synthetic_monitoring.Check
		class string
	}{
		"dns": {
			input: synthetic_monitoring.Check{
				Target: "127.0.0.1",
				Settings: synthetic_monitoring.CheckSettings{
					Dns: &synthetic_monitoring.DnsSettings{},
				},
			},
			class: "dns",
		},
		"dns_basic": {
			input: synthetic_monitoring.Check{
				Target:           "127.0.0.1",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Dns: &synthetic_monitoring.DnsSettings{},
				},
			},
			class: "dns_basic",
		},

		"http": {
			input: synthetic_monitoring.Check{
				Target: "http://127.0.0.1/",
				Settings: synthetic_monitoring.CheckSettings{
					Http: &synthetic_monitoring.HttpSettings{},
				},
			},
			class: "http",
		},
		"http_ssl": {
			input: synthetic_monitoring.Check{
				Target: "https://127.0.0.1/",
				Settings: synthetic_monitoring.CheckSettings{
					Http: &synthetic_monitoring.HttpSettings{},
				},
			},
			class: "http_ssl",
		},
		"http_basic": {
			input: synthetic_monitoring.Check{
				Target:           "http://127.0.0.1/",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Http: &synthetic_monitoring.HttpSettings{},
				},
			},
			class: "http_basic",
		},
		"http_ssl_basic": {
			input: synthetic_monitoring.Check{
				Target:           "https://127.0.0.1/",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Http: &synthetic_monitoring.HttpSettings{},
				},
			},
			class: "http_ssl_basic",
		},

		"ping": {
			input: synthetic_monitoring.Check{
				Target: "127.0.0.1",
				Settings: synthetic_monitoring.CheckSettings{
					Ping: &synthetic_monitoring.PingSettings{},
				},
			},
			class: "ping",
		},
		"ping_basic": {
			input: synthetic_monitoring.Check{
				Target:           "127.0.0.1",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Ping: &synthetic_monitoring.PingSettings{},
				},
			},
			class: "ping_basic",
		},

		"tcp": {
			input: synthetic_monitoring.Check{
				Target: "127.0.0.1:8080",
				Settings: synthetic_monitoring.CheckSettings{
					Tcp: &synthetic_monitoring.TcpSettings{},
				},
			},
			class: "tcp",
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
			class: "tcp_ssl",
		},
		"tcp_basic": {
			input: synthetic_monitoring.Check{
				Target:           "127.0.0.1:8080",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Tcp: &synthetic_monitoring.TcpSettings{},
				},
			},
			class: "tcp_basic",
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
			class: "tcp_ssl_basic",
		},
		"traceroute": {
			input: synthetic_monitoring.Check{
				Target:           "127.0.0.1",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Traceroute: &synthetic_monitoring.TracerouteSettings{
						MaxHops:    64,
						HopTimeout: 100,
					},
				},
			},
			class: "traceroute_basic",
		},
		"traceroute_basic": {
			input: synthetic_monitoring.Check{
				Target:           "127.0.0.1",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Traceroute: &synthetic_monitoring.TracerouteSettings{
						MaxHops:    64,
						HopTimeout: 100,
					},
				},
			},
			class: "traceroute_basic",
		},
		"scripted": {
			input: synthetic_monitoring.Check{
				Target:  "http://127.0.0.1/",
				Timeout: 2000,
				Settings: synthetic_monitoring.CheckSettings{
					Scripted: &synthetic_monitoring.ScriptedSettings{
						Script: []byte(`export default function() {}`),
					},
				},
			},
			class: "scripted",
		},
		"scripted_basic": {
			input: synthetic_monitoring.Check{
				Target:           "http://127.0.0.1/",
				Timeout:          2000,
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Scripted: &synthetic_monitoring.ScriptedSettings{
						Script: []byte(`export default function() {}`),
					},
				},
			},
			class: "scripted_basic",
		},
		"multihttp": {
			input: synthetic_monitoring.Check{
				Target:  "http://127.0.0.1/",
				Timeout: 2000,
				Settings: synthetic_monitoring.CheckSettings{
					Multihttp: &synthetic_monitoring.MultiHttpSettings{
						Entries: []*synthetic_monitoring.MultiHttpEntry{
							{
								Request: &synthetic_monitoring.MultiHttpEntryRequest{
									Method: synthetic_monitoring.HttpMethod_GET,
									Url:    "http://127.0.0.1/",
								},
							},
						},
					},
				},
			},
			class: "multihttp",
		},
		"multihttp_basic": {
			input: synthetic_monitoring.Check{
				Target:           "http://127.0.0.1/",
				Timeout:          2000,
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Multihttp: &synthetic_monitoring.MultiHttpSettings{
						Entries: []*synthetic_monitoring.MultiHttpEntry{
							{
								Request: &synthetic_monitoring.MultiHttpEntryRequest{
									Method: synthetic_monitoring.HttpMethod_GET,
									Url:    "http://127.0.0.1/",
								},
							},
						},
					},
				},
			},
			class: "multihttp_basic",
		},
		"grpc": {
			input: synthetic_monitoring.Check{
				Target: "127.0.0.1:9000",
				Settings: synthetic_monitoring.CheckSettings{
					Grpc: &synthetic_monitoring.GrpcSettings{},
				},
			},
			class: "grpc",
		},
		"grpc_ssl": {
			input: synthetic_monitoring.Check{
				Target: "127.0.0.1:9000",
				Settings: synthetic_monitoring.CheckSettings{
					Grpc: &synthetic_monitoring.GrpcSettings{
						Tls: true,
					},
				},
			},
			class: "grpc_ssl",
		},
		"grpc_basic": {
			input: synthetic_monitoring.Check{
				Target:           "127.0.0.1:9000",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Grpc: &synthetic_monitoring.GrpcSettings{},
				},
			},
			class: "grpc_basic",
		},
		"grpc_ssl_basic": {
			input: synthetic_monitoring.Check{
				Target:           "127.0.0.1:9000",
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Grpc: &synthetic_monitoring.GrpcSettings{
						Tls: true,
					},
				},
			},
			class: "grpc_ssl_basic",
		},
		"browser": {
			input: synthetic_monitoring.Check{
				Target:  "http://127.0.0.1/",
				Timeout: 2000,
				Settings: synthetic_monitoring.CheckSettings{
					Browser: &synthetic_monitoring.BrowserSettings{
						Script: []byte(`export default function() {}`),
					},
				},
			},
			class: "browser",
		},
		"browser_basic": {
			input: synthetic_monitoring.Check{
				Target:           "http://127.0.0.1/",
				Timeout:          2000,
				BasicMetricsOnly: true,
				Settings: synthetic_monitoring.CheckSettings{
					Browser: &synthetic_monitoring.BrowserSettings{
						Script: []byte(`export default function() {}`),
					},
				},
			},
			class: "browser_basic",
		},
	}
}

// TestGetActiveSeriesForCheck verifies that GetActiveSeriesForCheck
// returns the data in activeSeriesByCheckType. This makes sure that the
// function is applying the correct criteria to select the entry from
// the map. It also verifies that all the entries in that map are
// covered.
func TestGetActiveSeriesForCheck(t *testing.T) {
	testcases := getTestCases()

	// For know simply expect that every element in
	// activeSeriesByCheckType has a corresponding test case of the
	// same name. This ensures that if additional checks are added,
	// or new variants are introduced, they don't go unnoticed here.
	//
	// If more test cases are added, they can use names other than
	// the keys in activeSeriesByCheckType.
	for checkType := range activeSeriesByCheckType {
		_, found := testcases[checkType]
		require.Truef(t, found, "every element in activeSeriesByCheckType must be tested, missing %s", checkType)
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := GetActiveSeriesForCheck(tc.input)
			require.NoError(t, err)
			require.Equal(t, activeSeriesByCheckType[tc.class], actual)
		})
	}
}

func TestGetCheckAccountingClass(t *testing.T) {
	testcases := getTestCases()

	// See comment in TestGetActiveSeriesForCheck
	for checkType := range activeSeriesByCheckType {
		_, found := testcases[checkType]
		require.Truef(t, found, "every element in activeSeriesByCheckType must be tested, missing %s", checkType)
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := GetCheckAccountingClass(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.class, actual)
		})
	}
}

func TestGetAccountingClassInfo(t *testing.T) {
	info := GetAccountingClassInfo()

	for accountingClass, expectedSeries := range activeSeriesByCheckType {
		actual, found := info[accountingClass]
		require.True(t, found, "every element in activeSeriesByCheckType must be tested")
		require.Equal(t, expectedSeries, actual.Series)
		require.Equal(t, getTypeFromClass(accountingClass), actual.CheckType)
	}
}

// TestGetTypeFromClass verifies that the helper returns the correct
// type for the corresponding check.
func TestGetTypeFromClass(t *testing.T) {
	for name, tc := range getTestCases() {
		t.Run(name, func(t *testing.T) {
			expected := tc.input.Type()
			actual := getTypeFromClass(tc.class)
			require.Equal(t, expected, actual)
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
