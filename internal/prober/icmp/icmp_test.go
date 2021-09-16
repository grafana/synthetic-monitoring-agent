package icmp

import (
	"testing"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	"github.com/stretchr/testify/require"
)

func TestSettingsToModule(t *testing.T) {
	testcases := map[string]struct {
		input    sm.PingSettings
		expected config.Module
	}{
		"default": {
			input: sm.PingSettings{},
			expected: config.Module{
				Prober:  "ping",
				Timeout: 0,
				ICMP: config.ICMPProbe{
					IPProtocol:         "ip6",
					IPProtocolFallback: true,
				},
			},
		},
		"partial-settings": {
			input: sm.PingSettings{
				IpVersion:       1,
				SourceIpAddress: "0.0.0.0",
			},
			expected: config.Module{
				Prober:  "ping",
				Timeout: 0,
				ICMP: config.ICMPProbe{
					IPProtocol:         "ip4",
					IPProtocolFallback: false,
					SourceIPAddress:    "0.0.0.0",
				},
			},
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := settingsToModule(&testcase.input)
			require.Equal(t, &testcase.expected, &actual)
		})
	}
}
