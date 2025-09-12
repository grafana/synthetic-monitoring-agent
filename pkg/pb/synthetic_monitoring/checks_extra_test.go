package synthetic_monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

var testDebugOutput = flag.Bool("test.debug-output", false, "include test debug output")

func TestCheckValidate(t *testing.T) {
	type TestCase struct {
		input       Check
		expectError bool
	}

	testcases := map[string]TestCase{
		"invalid tenant": {
			input: Check{
				Id:        1,
				TenantId:  BadID,
				Target:    "127.0.0.1",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Ping: &PingSettings{},
				},
			},
			expectError: true,
		},
		"invalid label": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "127.0.0.1",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Ping: &PingSettings{},
				},
				Labels: []Label{{Name: "name ", Value: "value"}},
			},
			expectError: true,
		},
		"duplicate label names": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "127.0.0.1",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Ping: &PingSettings{},
				},
				Labels: []Label{
					{Name: "name", Value: "1"},
					{Name: "name", Value: "2"},
				},
			},
			expectError: true,
		},
		"duplicate label values": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "127.0.0.1",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Ping: &PingSettings{},
				},
				Labels: []Label{
					{Name: "name_1", Value: "1"},
					{Name: "name_2", Value: "1"},
				},
			},
			expectError: false,
		},
		"multiple settings": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "127.0.0.1",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Ping: &PingSettings{},
					Http: &HttpSettings{},
				},
				Labels: []Label{{Name: "name ", Value: "value"}},
			},
			expectError: true,
		},
		"valid timeout & frequency": { // test for case when frequency > max timeout
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "127.0.0.1",
				Job:       "job",
				Frequency: 60000, // 60 seconds
				Timeout:   5000,  // 5 seconds
				Probes:    []int64{1},
				Settings: CheckSettings{
					Ping: &PingSettings{},
				},
			},
			expectError: false,
		},
		"invalid timeout": { // issue #101
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "127.0.0.1",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1001, // timeout should be equal or less than frequency
				Probes:    []int64{1},
				Settings: CheckSettings{
					Ping: &PingSettings{},
				},
			},
			expectError: true,
		},
		"invalid HTTP target": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "ftp://example.org/",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Http: &HttpSettings{},
				},
				Labels: []Label{{Name: "name ", Value: "value"}},
			},
			expectError: true,
		},
		"valid proxy URL": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "http://example.org/",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Http: &HttpSettings{
						ProxyURL: "http://proxy.example.org/",
					},
				},
			},
			expectError: false,
		},
		"valid proxy URL and headers": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "http://example.org/",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Http: &HttpSettings{
						ProxyURL:            "http://proxy.example.org/",
						ProxyConnectHeaders: []string{"h1: v1", "h2:v2"},
					},
				},
			},
			expectError: false,
		},
		"proxy headers without url": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "http://example.org/",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Http: &HttpSettings{
						ProxyConnectHeaders: []string{"h1: v1", "h2:v2"},
					},
				},
			},
			expectError: true,
		},
		"valid HTTP check with long URL": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "http://example.org/" + strings.Repeat("x", maxValidLabelValueLength-len("http://example.org/")),
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Http: &HttpSettings{},
				},
			},
			expectError: false,
		},
		"valid multihttp check": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "https://example.org/",
				Job:       "job",
				Frequency: 60000,
				Timeout:   10000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Multihttp: &MultiHttpSettings{
						Entries: []*MultiHttpEntry{
							{
								Request: &MultiHttpEntryRequest{
									Url: "https://example.org/",
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		"valid multihttp variable": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "${variable}",
				Job:       "job",
				Frequency: 60000,
				Timeout:   10000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Multihttp: &MultiHttpSettings{
						Entries: []*MultiHttpEntry{
							{
								Request: &MultiHttpEntryRequest{
									Url: "${variable}",
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		"valid multihttp with variable in second url": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "https://www.example.com",
				Job:       "job",
				Frequency: 60000,
				Timeout:   10000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Multihttp: &MultiHttpSettings{
						Entries: []*MultiHttpEntry{
							{
								Request: &MultiHttpEntryRequest{
									Url: "https://www.example.com",
								},
							},
							{
								Request: &MultiHttpEntryRequest{
									Url: "${variable}",
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		"empty multihttp check": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "",
				Job:       "job",
				Frequency: 60000,
				Timeout:   10000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Multihttp: &MultiHttpSettings{
						Entries: []*MultiHttpEntry{
							{
								Request: &MultiHttpEntryRequest{
									Url: "https://example.org/",
								},
							},
						},
					},
				},
			},
			expectError: true,
		},
		"invalid multihttp URL": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "example.com", // this is fine
				Job:       "job",
				Frequency: 60000,
				Timeout:   10000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Multihttp: &MultiHttpSettings{
						Entries: []*MultiHttpEntry{
							{
								Request: &MultiHttpEntryRequest{
									Url: "example.com", // this is the problem
								},
							},
						},
					},
				},
			},
			expectError: true,
		},
		"multihttp target must not be an URL": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "example.com", // this is fine
				Job:       "job",
				Frequency: 60000,
				Timeout:   10000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Multihttp: &MultiHttpSettings{
						Entries: []*MultiHttpEntry{
							{
								Request: &MultiHttpEntryRequest{
									Url: "http://example.com",
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		"invalid multihttp second target": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "https://www.example.com",
				Job:       "job",
				Frequency: 60000,
				Timeout:   10000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Multihttp: &MultiHttpSettings{
						Entries: []*MultiHttpEntry{
							{
								Request: &MultiHttpEntryRequest{
									Url: "https://www.example.com",
								},
							},
							{
								Request: &MultiHttpEntryRequest{
									Url: "notavalidurlatall",
								},
							},
						},
					},
				},
			},
			expectError: true,
		},
		"valid multihttp variables everywhere": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "${variable}",
				Job:       "job",
				Frequency: 60000,
				Timeout:   10000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Multihttp: &MultiHttpSettings{
						Entries: []*MultiHttpEntry{
							{
								Request: &MultiHttpEntryRequest{
									Url: "${variable}",
									Headers: []*HttpHeader{
										{
											Name:  "Authorization",
											Value: "Bearer ${variable}",
										},
									},
									Body: &HttpRequestBody{
										ContentType:     "application/json",
										ContentEncoding: "gzip",
										Payload:         []byte("${variable}"),
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
	}

	// add trivial cases for all check types
	for _, checkType := range CheckTypeValues() {
		testcases["valid "+checkType.String()] = TestCase{
			input:       GetCheckInstance(checkType),
			expectError: false,
		}
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := testcase.input.Validate()
			checkError(t, testcase.expectError, err, testcase.input)
		})
	}
}

func TestCheckType(t *testing.T) {
	type testcase struct {
		input    Check
		expected CheckType
	}

	testcases := make(map[string]testcase)

	for _, checkType := range CheckTypeValues() {
		testcases[checkType.String()] = testcase{
			input:    GetCheckInstance(checkType),
			expected: checkType,
		}
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := testcase.input.Type()
			require.Equal(t, testcase.expected, actual)
		})
	}
}

func TestCheckClass(t *testing.T) {
	testcases := map[string]struct {
		input    Check
		expected CheckClass
	}{
		CheckTypeDns.String(): {
			input:    GetCheckInstance(CheckTypeDns),
			expected: CheckClass_PROTOCOL,
		},
		CheckTypeHttp.String(): {
			input:    GetCheckInstance(CheckTypeHttp),
			expected: CheckClass_PROTOCOL,
		},
		CheckTypePing.String(): {
			input:    GetCheckInstance(CheckTypePing),
			expected: CheckClass_PROTOCOL,
		},
		CheckTypeTcp.String(): {
			input:    GetCheckInstance(CheckTypeTcp),
			expected: CheckClass_PROTOCOL,
		},
		CheckTypeTraceroute.String(): {
			input:    GetCheckInstance(CheckTypeTraceroute),
			expected: CheckClass_PROTOCOL,
		},
		CheckTypeScripted.String(): {
			input:    GetCheckInstance(CheckTypeScripted),
			expected: CheckClass_SCRIPTED,
		},
		CheckTypeMultiHttp.String(): {
			input:    GetCheckInstance(CheckTypeMultiHttp),
			expected: CheckClass_SCRIPTED,
		},
		CheckTypeGrpc.String(): {
			input:    GetCheckInstance(CheckTypeGrpc),
			expected: CheckClass_PROTOCOL,
		},
		CheckTypeBrowser.String(): {
			input:    GetCheckInstance(CheckTypeBrowser),
			expected: CheckClass_BROWSER,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := testcase.input.Class()
			require.Equal(t, testcase.expected, actual)
		})
	}

	hasTest := make(map[CheckType]bool)
	for _, checkType := range CheckTypeValues() {
		hasTest[checkType] = false
	}

	for _, testcase := range testcases {
		hasTest[testcase.input.Type()] = true
	}

	for checkType, found := range hasTest {
		require.True(t, found, "missing test for check type %s", checkType)
	}
}

func TestCheckTypeString(t *testing.T) {
	testcases := map[string]struct {
		input    CheckType
		expected string
	}{
		"dns": {
			input:    CheckTypeDns,
			expected: "dns",
		},
		"http": {
			input:    CheckTypeHttp,
			expected: "http",
		},
		"ping": {
			input:    CheckTypePing,
			expected: "ping",
		},
		"tcp": {
			input:    CheckTypeTcp,
			expected: "tcp",
		},
		"traceroute": {
			input:    CheckTypeTraceroute,
			expected: "traceroute",
		},
		"scripted": {
			input:    CheckTypeScripted,
			expected: "scripted",
		},
		"multihttp": {
			input:    CheckTypeMultiHttp,
			expected: "multihttp",
		},
		"browser": {
			input:    CheckTypeBrowser,
			expected: "browser",
		},
	}

	for name, testcase := range testcases {
		actual := testcase.input.String()
		require.Equal(t, testcase.expected, actual, "testcase %s", name)
	}
}

func TestCheckTypeClass(t *testing.T) {
	testcases := map[string]struct {
		input    CheckType
		expected CheckClass
	}{
		CheckTypeDns.String(): {
			input:    CheckTypeDns,
			expected: CheckClass_PROTOCOL,
		},
		CheckTypeHttp.String(): {
			input:    CheckTypeHttp,
			expected: CheckClass_PROTOCOL,
		},
		CheckTypePing.String(): {
			input:    CheckTypePing,
			expected: CheckClass_PROTOCOL,
		},
		CheckTypeTcp.String(): {
			input:    CheckTypeTcp,
			expected: CheckClass_PROTOCOL,
		},
		CheckTypeTraceroute.String(): {
			input:    CheckTypeTraceroute,
			expected: CheckClass_PROTOCOL,
		},
		CheckTypeScripted.String(): {
			input:    CheckTypeScripted,
			expected: CheckClass_SCRIPTED,
		},
		CheckTypeMultiHttp.String(): {
			input:    CheckTypeMultiHttp,
			expected: CheckClass_SCRIPTED,
		},
		CheckTypeGrpc.String(): {
			input:    CheckTypeGrpc,
			expected: CheckClass_PROTOCOL,
		},
		CheckTypeBrowser.String(): {
			input:    CheckTypeBrowser,
			expected: CheckClass_BROWSER,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := testcase.input.Class()
			require.Equal(t, testcase.expected, actual)
		})
	}

	hasTest := make(map[CheckType]bool)
	for _, checkType := range CheckTypeValues() {
		hasTest[checkType] = false
	}

	for _, testcase := range testcases {
		hasTest[testcase.input] = true
	}

	for checkType, found := range hasTest {
		require.True(t, found, "missing test for check type %s", checkType)
	}
}

func TestValidateHost(t *testing.T) {
	testcases := map[string]struct {
		input       string
		expectError bool
	}{
		// valid hostnames
		"hostname": {
			input:       "grafana.com",
			expectError: false,
		},

		// invalid hostnames
		"invalid hostname": {
			input:       "grafana-com",
			expectError: true,
		},

		// valid IP addresses
		"IPv4": {
			input:       "1.2.3.4",
			expectError: false,
		},
		"IPv4 loopback": {
			input:       "127.0.0.1",
			expectError: false,
		},
		"IPv4 local multicast": {
			input:       "224.0.0.1",
			expectError: false,
		},
		"IPv4 control multicast": {
			input:       "224.0.1.1",
			expectError: false,
		},
		"IPv6": {
			input:       "ABCD:EF01:2345:6789:ABCD:EF01:2345:6789",
			expectError: false,
		},
		"IPv6 unicast": {
			input:       "2001:DB8:0:0:8:800:200C:417A",
			expectError: false,
		},
		"IPv6 link-local unicast": {
			input:       "FE80::1",
			expectError: false,
		},
		"IPv6 multicast": {
			input:       "FF01:0:0:0:0:0:0:101",
			expectError: false,
		},
		"IPv6 multicast all local nodes": {
			input:       "FF02::1",
			expectError: false,
		},
		"IPv6 loopback": {
			input:       "0:0:0:0:0:0:0:1",
			expectError: false,
		},
		"IPv6 loopback short": {
			input:       "::1",
			expectError: false,
		},
		"IPv6 unespecified short": {
			input:       "::",
			expectError: false,
		},
		"IPv4 as IPv6": {
			input:       "0:0:0:0:0:0:13.1.68.3",
			expectError: false,
		},
		"IPv4 in IPv6": {
			input:       "0:0:0:0:FF:FF:13.1.68.3",
			expectError: false,
		},
		"IPv4 as IPv6 short": {
			input:       "::13.1.68.3",
			expectError: false,
		},
		"IPv4 in IPv6 short": {
			input:       "::FFFF:13.1.68.3",
			expectError: false,
		},

		// invalid IP addresses
		"invalid IPv4": {
			input:       "0.0.0.256",
			expectError: true,
		},
		"invalid IPv6": {
			input:       "::10000",
			expectError: true,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := validateHost(testcase.input)
			checkError(t, testcase.expectError, err, testcase.input)
		})
	}
}

func TestValidateDnsTarget(t *testing.T) {
	testcases := map[string]struct {
		input       string
		expectError bool
	}{
		// valid hostnames
		"hostname": {
			input:       "grafana.com",
			expectError: false,
		},

		// localhost is valid
		"localhost": {
			input:       "localhost",
			expectError: false,
		},

		// localhost. is valid
		"localhost.": {
			input:       "localhost.",
			expectError: false,
		},

		// single label fully qualified dns name is valid
		"org.": {
			input:       "org.",
			expectError: false,
		},

		// multi-label dns name is valid
		"grafana.com.": {
			input:       "grafana.com.",
			expectError: false,
		},

		// single label is invalid
		"org": {
			input:       "org",
			expectError: true,
		},

		// For DNS entries, IP address is valid
		"127.0.0.1": {
			input:       "127.0.0.1",
			expectError: false,
		},

		// IP address disguised as multi-label fully qualified
		// dns name are also valid
		"127.0.0.1.": {
			input:       "127.0.0.1.",
			expectError: false,
		},

		// empty label
		"foo..bar": {
			input:       "foo..bar",
			expectError: true,
		},

		// label too long
		"foo.a62a.bar": {
			input:       "foo." + strings.Repeat("a", 64) + ".bar",
			expectError: true,
		},

		// zeroconf
		"_srv._tcp.example.org": {
			input:       "_srv._tcp.example.org",
			expectError: false,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := validateDnsTarget(testcase.input)
			checkError(t, testcase.expectError, err, testcase.input)
		})
	}
}

func TestCheckFQHN(t *testing.T) {
	genstr := func(n int) string {
		var sb strings.Builder
		sb.Grow(n)
		for i := 1; i <= n; i++ {
			_ = sb.WriteByte(byte('a' + (i % ('z' - 'a' + 1))))
		}
		return sb.String()
	}

	testcases := map[string]struct {
		input       string
		expectError bool
	}{
		"empty": {
			input:       "",
			expectError: true,
		},
		"too long": {
			input:       genstr(256),
			expectError: true,
		},
		"start with .": {
			input:       ".x",
			expectError: true,
		},
		"end with . 1": {
			input:       "x.",
			expectError: true,
		},
		"end with . 2": {
			input:       "x.y.",
			expectError: true,
		},
		"must have at least two labels": {
			input:       "x",
			expectError: true,
		},
		"label must start with letter or digit 1": {
			input:       "0.x",
			expectError: false,
		},
		"label must start with letter or digit 2": {
			input:       "-.x",
			expectError: true,
		},
		"label must start with letter or digit 3": {
			input:       "x.y",
			expectError: false,
		},
		"label must start with letter or digit 4": {
			input:       "1x.y",
			expectError: false,
		},
		"label must end with a letter or digit 1": {
			input:       "-.x",
			expectError: true,
		},
		"label must end with a letter or digit 2": {
			input:       "x.y",
			expectError: false,
		},
		"label must end with a letter or digit 3": {
			input:       "xy.z",
			expectError: false,
		},
		"label must end with a letter or digit 4": {
			input:       "x1.y",
			expectError: false,
		},
		"label must contain only letters, digits or dash 1": {
			input:       "x=y.z",
			expectError: true,
		},
		"label must contain only letters, digits or dash 2": {
			input:       "x-0.y-z",
			expectError: false,
		},
		"labels must be 63 characters or less 1": {
			input:       genstr(64) + ".x",
			expectError: true,
		},
		"labels must be 63 characters or less 2": {
			input:       genstr(63) + "." + genstr(63),
			expectError: false,
		},
		"valid, all lowercase": {
			input:       "grafana.com",
			expectError: false,
		},
		"valid, all uppercase": {
			input:       "GRAFANA.COM",
			expectError: false,
		},
		"valid, mixed case": {
			input:       "gRaFaNa.CoM",
			expectError: false,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := checkFQHN(testcase.input)
			checkError(t, testcase.expectError, err, testcase.input)
		})
	}
}

func TestValidateHostPort(t *testing.T) {
	testcases := map[string]struct {
		input       string
		expectError bool
	}{
		"trivial": {
			input:       "grafana.com:25",
			expectError: false,
		},
		"port 1": {
			input:       "grafana.com:1",
			expectError: false,
		},
		"port 65535": {
			input:       "grafana.com:65535",
			expectError: false,
		},

		"blank": {
			input:       "",
			expectError: true,
		},

		// invalid hosts
		"no host": {
			input:       ":25",
			expectError: true,
		},
		"invalid domain": {
			input:       "x:25",
			expectError: true,
		},
		"invalid host": {
			input:       "-.x:25",
			expectError: true,
		},

		// invalid ports
		"no port": {
			input:       "grafana.com",
			expectError: true,
		},
		"empty port": {
			input:       "grafana.com:",
			expectError: true,
		},
		"port zero": {
			input:       "grafana.com:0",
			expectError: true,
		},
		"negative port": {
			input:       "grafana.com:-1",
			expectError: true,
		},
		"port too large": {
			input:       "grafana.com:65536",
			expectError: true,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := validateHostPort(testcase.input)
			checkError(t, testcase.expectError, err, testcase.input)
		})
	}
}

func TestValidateHttpUrl(t *testing.T) {
	testcases := map[string]struct {
		input       string
		expectError bool
	}{
		"http": {
			input:       "http://example.org/",
			expectError: false,
		},
		"https": {
			input:       "https://example.org/",
			expectError: false,
		},
		"http port": {
			input:       "http://example.org:8000/",
			expectError: false,
		},
		"https port": {
			input:       "https://example.org:8443/",
			expectError: false,
		},
		"ipv4": {
			input:       "http://127.0.0.1/",
			expectError: false,
		},
		"ipv6": {
			input:       "http://[::1]/",
			expectError: false,
		},
		"ipv4 port": {
			input:       "http://127.0.0.1:80/",
			expectError: false,
		},
		"ipv6 port": {
			input:       "http://[::1]:80/",
			expectError: false,
		},
		"invalid scheme": {
			input:       "ftp://example.org/",
			expectError: true,
		},
		"with username": {
			input:       "http://user@example.org/",
			expectError: true,
		},
		"with username and password": {
			input:       "http://user:password@example.org/",
			expectError: true,
		},

		"blank": {
			input:       "",
			expectError: true,
		},
		"no host": {
			input:       "http://",
			expectError: true,
		},

		// these are covered by TestValidateHostPort
		"bad host": {
			input:       "http://test/",
			expectError: true,
		},
		"port too large": {
			input:       "http://example.org:65536/",
			expectError: true,
		},
		"port zero": {
			input:       "http://example.org:0/",
			expectError: true,
		},
		"negative port": {
			input:       "http://example.org:-1/",
			expectError: true,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := validateHttpUrl(testcase.input)
			checkError(t, testcase.expectError, err, testcase.input)
		})
	}
}

func TestValidateLabel(t *testing.T) {
	genString := func(n int) string {
		var s strings.Builder
		s.Grow(n)
		for range n {
			_ = s.WriteByte('x')
		}
		return s.String()
	}

	testcases := map[string]struct {
		input       Label
		expectError bool
	}{
		"trivial": {
			input:       Label{Name: "label", Value: "value"},
			expectError: false,
		},
		"name with underscore": {
			input:       Label{Name: "some_name", Value: "value"},
			expectError: false,
		},
		"name with leading underscore": {
			input:       Label{Name: "_name", Value: "value"},
			expectError: false,
		},
		"empty name": {
			input:       Label{Name: "", Value: "value"},
			expectError: true,
		},
		"invalid name": {
			input:       Label{Name: "foo@bar", Value: "value"},
			expectError: true,
		},
		"name with trailing blank": { // issue #99
			input:       Label{Name: "name ", Value: "value"},
			expectError: true,
		},
		"empty value": {
			input:       Label{Name: "name", Value: ""},
			expectError: true,
		},
		"long value": {
			input:       Label{Name: "name", Value: genString(MaxLabelValueLength)},
			expectError: false,
		},
		"value too long": {
			input:       Label{Name: "name", Value: genString(MaxLabelValueLength + 1)},
			expectError: true,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := testcase.input.Validate()
			checkError(t, testcase.expectError, err, testcase.input)
		})
	}
}

func TestHttpSettingsValidate(t *testing.T) {
	testcases := map[string]struct {
		input       HttpSettings
		expectError bool
	}{
		"trivial": {
			input:       HttpSettings{},
			expectError: false,
		},
		"valid headers": {
			input: HttpSettings{
				Headers: []string{"header: value"},
			},
			expectError: false,
		},
		"no value is OK": {
			input: HttpSettings{
				Headers: []string{"header:"},
			},
			expectError: false,
		},
		"empty header is not OK": {
			input: HttpSettings{
				Headers: []string{": value"},
			},
			expectError: true,
		},
		"empty": {
			input: HttpSettings{
				Headers: []string{""},
			},
			expectError: true,
		},
		"no colon": {
			input: HttpSettings{
				Headers: []string{"header"},
			},
			expectError: true,
		},
		"multiple colons": {
			input: HttpSettings{
				Headers: []string{"origin:https://www.grafana.com"},
			},
			expectError: false,
		},
		"invalid name": {
			input: HttpSettings{
				Headers: []string{"hea;der: value"},
			},
			expectError: true,
		},
		"empty header/value": {
			input: HttpSettings{
				Headers: []string{":"},
			},
			expectError: true,
		},
		"blank header/value": {
			input: HttpSettings{
				Headers: []string{" : "},
			},
			expectError: true,
		},
		"non-ascii value": {
			input: HttpSettings{
				Headers: []string{"header: यूनिकोड टेक्स्ट"},
			},
			expectError: false,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := testcase.input.Validate()
			checkError(t, testcase.expectError, err, testcase.input)
		})
	}
}

func TestCompressionAlgorithmMarshal(t *testing.T) {
	type testStruct struct {
		Compression CompressionAlgorithm `json:"compression,omitempty"`
	}

	testcases := map[string]struct {
		unserialized testStruct
		serialized   []byte
	}{
		"none": {
			unserialized: testStruct{
				Compression: CompressionAlgorithm_none,
			},
			serialized: []byte(`{}`),
		},
		"gzip": {
			unserialized: testStruct{
				Compression: CompressionAlgorithm_gzip,
			},
			serialized: []byte(`{"compression":"gzip"}`),
		},
		"br": {
			unserialized: testStruct{
				Compression: CompressionAlgorithm_br,
			},
			serialized: []byte(`{"compression":"br"}`),
		},
		"deflate": {
			unserialized: testStruct{
				Compression: CompressionAlgorithm_deflate,
			},
			serialized: []byte(`{"compression":"deflate"}`),
		},
		"identity": {
			unserialized: testStruct{
				Compression: CompressionAlgorithm_identity,
			},
			serialized: []byte(`{"compression":"identity"}`),
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := json.Marshal(tc.unserialized)
			require.NoError(t, err)
			require.Equal(t, tc.serialized, actual)
		})
	}
}

func TestCompressionAlgorithmUnmarshal(t *testing.T) {
	type testStruct struct {
		Compression CompressionAlgorithm `json:"compression,omitempty"`
	}

	testcases := map[string]struct {
		unserialized testStruct
		serialized   []byte
	}{
		"none": {
			unserialized: testStruct{
				Compression: CompressionAlgorithm_none,
			},
			serialized: []byte(`{}`),
		},
		"empty": {
			unserialized: testStruct{
				Compression: CompressionAlgorithm_none,
			},
			serialized: []byte(`{"compression":""}`),
		},
		"null": {
			unserialized: testStruct{
				Compression: CompressionAlgorithm_none,
			},
			serialized: []byte(`{"compression":null}`),
		},
		"gzip": {
			unserialized: testStruct{
				Compression: CompressionAlgorithm_gzip,
			},
			serialized: []byte(`{"compression":"gzip"}`),
		},
		"br": {
			unserialized: testStruct{
				Compression: CompressionAlgorithm_br,
			},
			serialized: []byte(`{"compression":"br"}`),
		},
		"deflate": {
			unserialized: testStruct{
				Compression: CompressionAlgorithm_deflate,
			},
			serialized: []byte(`{"compression":"deflate"}`),
		},
		"identity": {
			unserialized: testStruct{
				Compression: CompressionAlgorithm_identity,
			},
			serialized: []byte(`{"compression":"identity"}`),
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var actual testStruct
			err := json.Unmarshal(tc.serialized, &actual)
			require.NoError(t, err)
			require.Equal(t, tc.unserialized, actual)
		})
	}
}

func checkError(t *testing.T, expectError bool, err error, input any) {
	t.Helper()

	switch {
	case expectError && err == nil:
		// unexpected success
		t.Errorf("expecting failure for input %q, but got success", input)

	case !expectError && err != nil:
		// unexpected failure
		t.Errorf("expecting success for input %q, but got failure: %s", input, err.Error())

	case expectError && err != nil:
		// expected failure
		if *testDebugOutput {
			t.Logf("expecting failure for input %q, got failure: %s", input, err.Error())
		}

	case !expectError && err == nil:
		// expected success
		if *testDebugOutput {
			t.Logf("expecting success for input %q, got success", input)
		}
	}
}

type TestCases[T any] map[string]struct {
	input       T
	expectError bool
}

func testValidate[T validatable](t *testing.T, testcases TestCases[T]) {
	t.Helper()

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := testcase.input.Validate()
			if testcase.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

type testValidatable struct {
	err error
}

func (v testValidatable) Validate() error {
	return v.err
}

// TestValidateCollection vefifies that validateCollection calls `Validate` on
// each member of a collection of `Validatable` objects.
func TestValidateCollection(t *testing.T) {
	invalid := errors.New("invalid")

	testcases := map[string]struct {
		input       []testValidatable
		expectError bool
	}{
		"empty": {
			input:       nil,
			expectError: false,
		},
		"one valid": {
			input:       []testValidatable{{err: nil}},
			expectError: false,
		},
		"one invalid": {
			input:       []testValidatable{{err: errors.New("invalid")}},
			expectError: true,
		},
		"two valid": {
			input:       []testValidatable{{err: nil}, {err: nil}},
			expectError: false,
		},
		"two invalid": {
			input:       []testValidatable{{err: invalid}, {err: invalid}},
			expectError: true,
		},
		"mixed": {
			input:       []testValidatable{{err: nil}, {err: invalid}},
			expectError: true,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := validateCollection(testcase.input)
			if testcase.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestHttpHeaderValidate(t *testing.T) {
	testValidate(t, TestCases[HttpHeader]{
		"valid": {
			input:       HttpHeader{Name: "name", Value: "value"},
			expectError: false,
		},
		"an empty value is valid": {
			input:       HttpHeader{Name: "xyz", Value: ""},
			expectError: false,
		},
		"an empty name is invalid": {
			input:       HttpHeader{Name: "", Value: "xxx"},
			expectError: true,
		},
		"invalid header name": {
			input:       HttpHeader{Name: "x y z", Value: ""},
			expectError: true,
		},
	})
}

func TestQueryFieldValidate(t *testing.T) {
	testValidate(t, TestCases[QueryField]{
		"valid": {
			input:       QueryField{Name: "name", Value: "value"},
			expectError: false,
		},
		"an empty value is valid": {
			input:       QueryField{Name: "xyz", Value: ""},
			expectError: false,
		},
		"an empty name is invalid": {
			input:       QueryField{Name: "", Value: "xxx"},
			expectError: true,
		},
		"a name with spaces is valid": {
			input:       QueryField{Name: "x y z", Value: ""},
			expectError: false,
		},
	})
}

func TestHttpRequestBodyValidate(t *testing.T) {
	testValidate(t, TestCases[*HttpRequestBody]{
		"nil is valid": {
			input:       nil,
			expectError: false,
		},
		"no content type is invalid": {
			input: &HttpRequestBody{
				ContentType:     "",
				ContentEncoding: "identity",
				Payload:         []byte{42},
			},
			expectError: true,
		},
		"empty content encoding is valid": {
			input: &HttpRequestBody{
				ContentType:     "text/plain",
				ContentEncoding: "",
				Payload:         []byte{42},
			},
			expectError: false,
		},
		"content type must be a valid media type": {
			input: &HttpRequestBody{
				// media types should not have space before the /
				ContentType:     "json file",
				ContentEncoding: "",
				Payload:         []byte{42},
			},
			expectError: true,
		},
		"empty payload is valid": {
			input: &HttpRequestBody{
				ContentType:     "text/plain",
				ContentEncoding: "identity",
				Payload:         []byte{},
			},
			expectError: false,
		},
	})
}

func TestMultiHttpEntryAssertionValidate(t *testing.T) {
	testValidate(t, TestCases[*MultiHttpEntryAssertion]{
		"nil is valid": {
			input:       nil,
			expectError: false,
		},
		"zero value is invalid": {
			// The zero value is a text assertion with "body" as the
			// subject and "contains" as the condition. The
			// "value" is empty, and that is not allowed.
			input:       &MultiHttpEntryAssertion{},
			expectError: true,
		},
		"text+body+contains with valid value": {
			input: &MultiHttpEntryAssertion{
				Type:      MultiHttpEntryAssertionType_TEXT,
				Subject:   MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Condition: MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Value:     "foobar",
			},
			expectError: false,
		},
		"text does not allow expressions": {
			input: &MultiHttpEntryAssertion{
				Type:       MultiHttpEntryAssertionType_TEXT,
				Subject:    MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Condition:  MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Expression: "foo",
				Value:      "bar",
			},
			expectError: true,
		},
		"json path value with valid value": {
			input: &MultiHttpEntryAssertion{
				Type:       MultiHttpEntryAssertionType_JSON_PATH_VALUE,
				Condition:  MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Expression: "$.data",
				Value:      "foo",
			},
			expectError: false,
		},
		"json path value does not allow for setting the subject": {
			input: &MultiHttpEntryAssertion{
				Type:       MultiHttpEntryAssertionType_JSON_PATH_VALUE,
				Subject:    MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS, // invalid
				Condition:  MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Expression: "$.data",
				Value:      "foo",
			},
			expectError: true,
		},
		"valid json path assertion": {
			input: &MultiHttpEntryAssertion{
				Type:       MultiHttpEntryAssertionType_JSON_PATH_ASSERTION,
				Expression: "$.foo",
			},
			expectError: false,
		},
		"json path assertion does not allow subject": {
			input: &MultiHttpEntryAssertion{
				Type:       MultiHttpEntryAssertionType_JSON_PATH_ASSERTION,
				Subject:    MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS, // invalid
				Expression: "$.foo",
			},
			expectError: true,
		},
		"json path assertion does not allow condition": {
			input: &MultiHttpEntryAssertion{
				Type:       MultiHttpEntryAssertionType_JSON_PATH_ASSERTION,
				Condition:  1, // invalid
				Expression: "$.foo",
			},
			expectError: true,
		},
		"json path assertion does not allow value": {
			input: &MultiHttpEntryAssertion{
				Type:       MultiHttpEntryAssertionType_JSON_PATH_ASSERTION,
				Value:      "bar", // invalid
				Expression: "$.foo",
			},
			expectError: true,
		},
		"valid regexp assertion": {
			input: &MultiHttpEntryAssertion{
				Type:       MultiHttpEntryAssertionType_REGEX_ASSERTION,
				Subject:    MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Expression: "foo",
			},
			expectError: false,
		},
		"regexp assertion does not allow value": {
			input: &MultiHttpEntryAssertion{
				Type:       MultiHttpEntryAssertionType_REGEX_ASSERTION,
				Subject:    MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Expression: "foo",
				Value:      "bar", // invalid
			},
			expectError: true,
		},
		"regexp assertion does not allow condition": {
			input: &MultiHttpEntryAssertion{
				Type:       MultiHttpEntryAssertionType_REGEX_ASSERTION,
				Subject:    MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Expression: "foo",
				Condition:  1, // invalid
			},
			expectError: true,
		},
	})
}

func TestHttpRegexFields(t *testing.T) {
	testValidate(t, TestCases[*HttpSettings]{
		"body matches regexp parses": {
			input: &HttpSettings{
				FailIfBodyMatchesRegexp: []string{".*good stuff.*"},
			},
			expectError: false,
		},
		"body matches invalid regexp errors": {
			input: &HttpSettings{
				FailIfBodyMatchesRegexp: []string{"*good stuff*"},
			},
			expectError: true,
		},
		"body not matches regexp parses": {
			input: &HttpSettings{
				FailIfBodyNotMatchesRegexp: []string{".*good stuff.*"},
			},
			expectError: false,
		},
		"body not matches invalid regexp errors": {
			input: &HttpSettings{
				FailIfBodyNotMatchesRegexp: []string{"*good stuff*"},
			},
			expectError: true,
		},
		"header matches regexp parses": {
			input: &HttpSettings{
				FailIfHeaderMatchesRegexp: []HeaderMatch{{Regexp: ".*good stuff.*"}},
			},
			expectError: false,
		},
		"header matches invalid regexp errors": {
			input: &HttpSettings{
				FailIfHeaderMatchesRegexp: []HeaderMatch{{Regexp: "*good stuff*"}},
			},
			expectError: true,
		},
		"header not matches regexp parses": {
			input: &HttpSettings{
				FailIfHeaderNotMatchesRegexp: []HeaderMatch{{Regexp: ".*good stuff.*"}},
			},
			expectError: false,
		},
		"header not matches invalid regexp errors": {
			input: &HttpSettings{
				FailIfHeaderNotMatchesRegexp: []HeaderMatch{{Regexp: "*good stuff*"}},
			},
			expectError: true,
		},
	})
}

func TestMultiHttpEntryVariableValidate(t *testing.T) {
	testValidate(t, TestCases[*MultiHttpEntryVariable]{
		"zero value": {
			// The zero value is invalid because a name and an
			// expression are required.
			input:       &MultiHttpEntryVariable{},
			expectError: true,
		},
		"json path": {
			input: &MultiHttpEntryVariable{
				Type:       MultiHttpEntryVariableType_JSON_PATH,
				Name:       "foo",
				Expression: "$.bar",
			},
			expectError: false,
		},
		"json path without a name is invalid": {
			input: &MultiHttpEntryVariable{
				Type:       MultiHttpEntryVariableType_JSON_PATH,
				Name:       "",
				Expression: "$.bar",
			},
			expectError: true,
		},
		"json path without an expression is invalid": {
			input: &MultiHttpEntryVariable{
				Type:       MultiHttpEntryVariableType_JSON_PATH,
				Name:       "foo",
				Expression: "",
			},
			expectError: true,
		},
		"json path with an attribute is invalid": {
			input: &MultiHttpEntryVariable{
				Type:       MultiHttpEntryVariableType_JSON_PATH,
				Name:       "foo",
				Expression: "bar",
				Attribute:  "baz",
			},
			expectError: true,
		},
		"regexp": {
			input: &MultiHttpEntryVariable{
				Type:       MultiHttpEntryVariableType_REGEX,
				Name:       "foo",
				Expression: "bar",
			},
			expectError: false,
		},
		"regexp without a name is invalid": {
			input: &MultiHttpEntryVariable{
				Type:       MultiHttpEntryVariableType_REGEX,
				Name:       "",
				Expression: "bar",
			},
			expectError: true,
		},
		"regexp without an expression is invalid": {
			input: &MultiHttpEntryVariable{
				Type:       MultiHttpEntryVariableType_REGEX,
				Name:       "foo",
				Expression: "",
			},
			expectError: true,
		},
		"regexp with an attribute is invalid": {
			input: &MultiHttpEntryVariable{
				Type:       MultiHttpEntryVariableType_REGEX,
				Name:       "foo",
				Expression: "bar",
				Attribute:  "baz",
			},
			expectError: true,
		},
		"css selector": {
			input: &MultiHttpEntryVariable{
				Type:       MultiHttpEntryVariableType_CSS_SELECTOR,
				Name:       "foo",
				Expression: "bar",
			},
			expectError: false,
		},
		"css selector without a name is invalid": {
			input: &MultiHttpEntryVariable{
				Type:       MultiHttpEntryVariableType_CSS_SELECTOR,
				Name:       "",
				Expression: "bar",
			},
			expectError: true,
		},
		"css selector without an expression is invalid": {
			input: &MultiHttpEntryVariable{
				Type:       MultiHttpEntryVariableType_CSS_SELECTOR,
				Name:       "foo",
				Expression: "",
			},
			expectError: true,
		},
		"css selector with attribute": {
			input: &MultiHttpEntryVariable{
				Type:       MultiHttpEntryVariableType_CSS_SELECTOR,
				Name:       "foo",
				Expression: "bar",
				Attribute:  "baz",
			},
			expectError: false,
		},
	})
}

func TestMultiHttpEntryRequestValidate(t *testing.T) {
	testValidate(t, TestCases[*MultiHttpEntryRequest]{
		"nil": {
			input:       nil,
			expectError: false,
		},
		"zero value": {
			// The zero value is invalid because it doesn't have a URL
			input:       &MultiHttpEntryRequest{},
			expectError: true,
		},
		"valid": {
			input: &MultiHttpEntryRequest{
				Method: HttpMethod_GET,
				Url:    "http://example.com",
			},
			expectError: false,
		},
		"invalid headers": {
			input: &MultiHttpEntryRequest{
				Method: HttpMethod_GET,
				Url:    "http://example.com",
				Headers: []*HttpHeader{
					{Name: "", Value: ""},
				},
			},
			expectError: true,
		},
		"invalid query": {
			input: &MultiHttpEntryRequest{
				Method: HttpMethod_GET,
				Url:    "http://example.com",
				QueryFields: []*QueryField{
					{Name: "", Value: ""},
				},
			},
			expectError: true,
		},
		"invalid body": {
			input: &MultiHttpEntryRequest{
				Method: HttpMethod_GET,
				Url:    "http://example.com",
				Body: &HttpRequestBody{
					ContentType:     "", // ContentType must not be empty
					ContentEncoding: "",
					Payload:         []byte(""),
				},
			},
			expectError: true,
		},
	})
}

// TestMultiHttpSettingsValidate verifies that MultiHttpSettings.Validate
// performs the expected validations.
//
// Other tests validate various invalid values. This test verifies that a
// single invalid value is reported in order to verify that the inner
// validations are happening.
func TestMultiHttpSettingsValidate(t *testing.T) {
	createEntries := func(n int, method HttpMethod, url string) []*MultiHttpEntry {
		entries := make([]*MultiHttpEntry, n)

		for i := range n {
			entries[i] = &MultiHttpEntry{
				Request: &MultiHttpEntryRequest{
					Method: method,
					Url:    url,
				},
			}
		}

		return entries
	}

	testValidate(t, TestCases[*MultiHttpSettings]{
		"zero value": {
			// the zero value for MultiHttpSettings is not usable
			// because at least one entry is required.
			input:       &MultiHttpSettings{},
			expectError: true,
		},
		"valid": {
			input: &MultiHttpSettings{
				Entries: createEntries(1, HttpMethod_GET, "http://example.com"),
			},
			expectError: false,
		},
		"many entries": {
			input: &MultiHttpSettings{
				Entries: createEntries(MaxMultiHttpTargets, HttpMethod_GET, "http://example.com"),
			},
			expectError: false,
		},
		"too many entries": {
			input: &MultiHttpSettings{
				Entries: createEntries(MaxMultiHttpTargets+1, HttpMethod_GET, "http://example.com"),
			},
			expectError: true,
		},
		"invalid request": {
			input: &MultiHttpSettings{
				Entries: []*MultiHttpEntry{
					{
						Request: &MultiHttpEntryRequest{
							Method: HttpMethod_GET,
							Url:    "",
						},
					},
				},
			},
			expectError: true,
		},
		"invalid assertion": {
			input: &MultiHttpSettings{
				Entries: []*MultiHttpEntry{
					{
						Request: &MultiHttpEntryRequest{
							Method: HttpMethod_GET,
							Url:    "http://example.com",
						},
						Assertions: []*MultiHttpEntryAssertion{
							{
								Type: -1,
							},
						},
					},
				},
			},
			expectError: true,
		},
		"invalid variable": {
			input: &MultiHttpSettings{
				Entries: []*MultiHttpEntry{
					{
						Request: &MultiHttpEntryRequest{
							Method: HttpMethod_GET,
							Url:    "http://example.com",
						},
						Variables: []*MultiHttpEntryVariable{
							{
								Type: -1,
							},
						},
					},
				},
			},
			expectError: true,
		},
	})
}

func TestInClosedRange(t *testing.T) {
	testcases := map[string]struct {
		value    int64
		lower    int64
		upper    int64
		expected bool
	}{
		"too low":     {value: 0, lower: 1, upper: 5, expected: false},
		"lower bound": {value: 1, lower: 1, upper: 5, expected: true},
		"in range":    {value: 3, lower: 1, upper: 5, expected: true},
		"upper bound": {value: 5, lower: 1, upper: 5, expected: true},
		"too high":    {value: 6, lower: 1, upper: 5, expected: false},
	}

	for name, tc := range testcases {
		actual := inClosedRange(tc.value, tc.lower, tc.upper)
		require.Equalf(t, tc.expected, actual, `%s`, name)
	}
}

func TestGetCheckInstance(t *testing.T) {
	for _, checkType := range CheckTypeValues() {
		check := GetCheckInstance(checkType)
		require.NotNil(t, check)
		require.Equal(t, checkType, check.Type())
		require.NoError(t, check.Validate())
	}
}

func requireRemoteInfoFields(t *testing.T) {
	t.Helper()

	// The expected fields in the RemoteInfo struct. It's necessary to
	// assert this list so that if the implementation ever changes, we can
	// go update the MarshalZerologObject method accordingly.
	expectedFields := []string{
		"Name",
		"Url",
		"Username",
		"Password",
	}

	remoteInfoType := reflect.TypeOf(RemoteInfo{})
	actualFields := make([]string, 0, remoteInfoType.NumField())

	for i := range remoteInfoType.NumField() {
		field := remoteInfoType.Field(i)
		if field.IsExported() {
			actualFields = append(actualFields, field.Name)
		}
	}

	require.ElementsMatch(t, expectedFields, actualFields,
		"RemoteInfo struct fields have changed. Update the expected fields list and review any code that depends on the field order.")
}

func TestRemoteInfoMarshalZerologObject(t *testing.T) {
	requireRemoteInfoFields(t)

	remoteInfo := RemoteInfo{
		Name:     "the name",
		Url:      "https://example.com",
		Username: "the username",
		Password: "the password",
	}

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	logger.Info().Interface("remote_info", remoteInfo).Send()

	// Note that the order of the expected fields is fixed. If the
	// implementation changes, this test will break.
	expected := `{"level":"info","remote_info":{"name":"the name","url":"https://example.com","username":"the username","password":"<encrypted>"}}` + "\n"
	actual := buf.String()

	require.Equal(t, expected, actual)
}

func TestRemoteInfoMarshalJSON(t *testing.T) {
	requireRemoteInfoFields(t)

	remoteInfo := RemoteInfo{
		Name:     "the name",
		Url:      "https://example.com",
		Username: "the username",
		Password: "the password",
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(&remoteInfo)
	require.NoError(t, err)

	expected := `{"name":"the name","url":"https://example.com","username":"the username","password":"\u003cencrypted\u003e"}`
	actual := strings.TrimSpace(buf.String())
	require.Equal(t, expected, actual, "JSON encoding of RemoteInfo did not match expected output")
}
