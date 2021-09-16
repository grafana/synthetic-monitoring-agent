package dns

import (
	"testing"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	"github.com/stretchr/testify/require"
)

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
	}

	for name, testcase := range testcases {
		target := "www.grafana.com"
		t.Run(name, func(t *testing.T) {
			actual := settingsToModule(&testcase.input, target)
			require.Equal(t, &testcase.expected, &actual)
		})
	}
}
