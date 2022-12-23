package synthetic_monitoring

import (
	"encoding/json"
	"flag"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var testDebugOutput = flag.Bool("test.debug-output", false, "include test debug output")

func TestCheckValidate(t *testing.T) {
	testcases := map[string]struct {
		input       Check
		expectError bool
	}{
		"trivial ping": {
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
			},
			expectError: false,
		},
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
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := testcase.input.Validate()
			checkError(t, testcase.expectError, err, testcase.input)
		})
	}
}

func TestCheckType(t *testing.T) {
	testcases := map[string]struct {
		input    Check
		expected CheckType
	}{
		"dns": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "www.example.org",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Dns: &DnsSettings{
						Server: "127.0.0.1",
					},
				},
			},
			expected: CheckTypeDns,
		},
		"http": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "http://www.example.org",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Http: &HttpSettings{},
				},
			},
			expected: CheckTypeHttp,
		},
		"ping": {
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
			},
			expected: CheckTypePing,
		},
		"tcp": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "127.0.0.1:9000",
				Job:       "job",
				Frequency: 1000,
				Timeout:   1000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Tcp: &TcpSettings{},
				},
			},
			expected: CheckTypeTcp,
		},
		"traceroute": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "127.0.0.1",
				Job:       "job",
				Frequency: 120000,
				Timeout:   30000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					Traceroute: &TracerouteSettings{},
				},
			},
			expected: CheckTypeTraceroute,
		},

		"k6": {
			input: Check{
				Id:        1,
				TenantId:  1,
				Target:    "http://www.example.org",
				Job:       "job",
				Frequency: 10000,
				Timeout:   10000,
				Probes:    []int64{1},
				Settings: CheckSettings{
					K6: &K6Settings{
						Script: []byte("// test"),
					},
				},
			},
			expected: CheckTypeK6,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := testcase.input.Validate()
			checkError(t, false, err, testcase.input)
			if err != nil {
				return
			}

			actual := testcase.input.Type()
			if testcase.expected != actual {
				t.Errorf(`expecting %[1]d (%[1]s) for input %[3]q, but got %[2]d (%[2]s)`, testcase.expected, actual, &testcase.input)
			}
		})
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
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := testcase.input.String()
			if testcase.expected != actual {
				t.Errorf(`expecting %s for input %q, but got %s`, testcase.expected, &testcase.input, actual)
			}
		})
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
		for i := 0; i < n; i++ {
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

func checkError(t *testing.T, expectError bool, err error, input interface{}) {
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
