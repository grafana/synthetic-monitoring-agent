package dns

import (
	"testing"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	name := Prober.Name(Prober{})
	require.Equal(t, name, "dns")
}

func TestNewProber(t *testing.T) {
	testcases := map[string]struct {
		input    sm.Check
		expected Prober
	}{
		"default": {
			input: sm.Check{
				Target: "www.grafana.com",
				Settings: sm.CheckSettings{
					Dns: &sm.DnsSettings{},
				},
			},
			expected: Prober{
				config: config.Module{
					Prober:  "dns",
					Timeout: 0,
					DNS: config.DNSProbe{
						IPProtocol:         "ip6",
						IPProtocolFallback: true,
						TransportProtocol:  "tcp",
						QueryName:          "www.grafana.com",
						QueryType:          "ANY",
					},
				},
			},
		},
		"no-settings": {
			input: sm.Check{
				Target: "www.grafana.com",
				Settings: sm.CheckSettings{
					Dns: nil,
				},
			},
			expected: Prober{},
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := NewProber(testcase.input)
			require.Equal(t, &testcase.expected, &actual)
			if name == "no-settings" {
				require.NotNil(t, err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestSettingsToModule(t *testing.T) {
	testcases := map[string]struct {
		input    sm.DnsSettings
		expected config.Module
	}{
		"default": {
			input: sm.DnsSettings{},
			expected: config.Module{
				Prober:  "dns",
				Timeout: 0,
				DNS: config.DNSProbe{
					IPProtocol:         "ip6",
					IPProtocolFallback: true,
					TransportProtocol:  "tcp",
					QueryName:          "www.grafana.com",
					QueryType:          "ANY",
				},
			},
		},
		"partial-settings": {
			input: sm.DnsSettings{
				RecordType: 4,
				Protocol:   1,
			},
			expected: config.Module{
				Prober:  "dns",
				Timeout: 0,
				DNS: config.DNSProbe{
					IPProtocol:         "ip6",
					IPProtocolFallback: true,
					TransportProtocol:  "udp",
					QueryName:          "www.grafana.com",
					QueryType:          "MX",
				},
			},
		},
		"validations": {
			input: sm.DnsSettings{
				RecordType: 4,
				Protocol:   1,
				ValidateAnswer: &sm.DNSRRValidator{
					FailIfMatchesRegexp:    []string{"test"},
					FailIfNotMatchesRegexp: []string{"not test"},
				},
				ValidateAuthority: &sm.DNSRRValidator{
					FailIfMatchesRegexp:    []string{"test"},
					FailIfNotMatchesRegexp: []string{"not test"},
				},
				ValidateAdditional: &sm.DNSRRValidator{
					FailIfMatchesRegexp:    []string{"test"},
					FailIfNotMatchesRegexp: []string{"not test"},
				},
			},
			expected: config.Module{
				Prober:  "dns",
				Timeout: 0,
				DNS: config.DNSProbe{
					IPProtocol:         "ip6",
					IPProtocolFallback: true,
					TransportProtocol:  "udp",
					QueryName:          "www.grafana.com",
					QueryType:          "MX",
					ValidateAnswer: config.DNSRRValidator{
						FailIfMatchesRegexp:    []string{"test"},
						FailIfNotMatchesRegexp: []string{"not test"},
					},
					ValidateAuthority: config.DNSRRValidator{
						FailIfMatchesRegexp:    []string{"test"},
						FailIfNotMatchesRegexp: []string{"not test"},
					},
					ValidateAdditional: config.DNSRRValidator{
						FailIfMatchesRegexp:    []string{"test"},
						FailIfNotMatchesRegexp: []string{"not test"},
					},
				},
			},
		},
	}

	for name, testcase := range testcases {
		target := "www.grafana.com"
		t.Run(name, func(t *testing.T) {
			actual := settingsToModule(&testcase.input, target)
			require.Equal(t, &testcase.expected, &actual)
		})
	}
}
