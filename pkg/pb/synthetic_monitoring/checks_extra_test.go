package synthetic_monitoring

import (
	"flag"
	"strings"
	"testing"
)

var testDebugOutput = flag.Bool("test.debug-output", false, "include test debug output")

func TestCheckValidate(t *testing.T) {
	testcases := map[string]struct {
		input       Check
		expectError bool
	}{
		"trivial ping": {
			input: Check{
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
				TenantId:  -1,
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
		"multiple settings": {
			input: Check{
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
		"invalid timeout": { // issue #101
			input: Check{
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
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := testcase.input.Validate()
			checkError(t, testcase.expectError, err, testcase.input)
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

func TestValidateLabel(t *testing.T) {
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
			input:       Label{Name: "name", Value: "12345678901234567890123456789012"},
			expectError: false,
		},
		"value too long": {
			input:       Label{Name: "name", Value: "123456789012345678901234567890123"},
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
