package tcp

import (
	"context"
	"io"
	"testing"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	name := Prober.Name(Prober{})
	require.Equal(t, name, "tcp")
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
					Tcp: &sm.TcpSettings{},
				},
			},
			expected: Prober{
				config: config.Module{
					Prober:  "tcp",
					Timeout: 0,
					TCP: config.TCPProbe{
						IPProtocol:         "ip6",
						IPProtocolFallback: true,
						QueryResponse:      []config.QueryResponse{},
					},
				},
			},
		},
		"no-settings": {
			input: sm.Check{
				Target: "www.grafana.com",
				Settings: sm.CheckSettings{
					Tcp: nil,
				},
			},
			expected: Prober{},
		},
	}

	for name, testcase := range testcases {
		ctx := context.TODO()
		logger := zerolog.New(io.Discard)
		t.Run(name, func(t *testing.T) {
			actual, err := NewProber(ctx, testcase.input, logger)
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
		input    sm.TcpSettings
		expected config.Module
	}{
		"default": {
			input: sm.TcpSettings{},
			expected: config.Module{
				Prober:  "tcp",
				Timeout: 0,
				TCP: config.TCPProbe{
					IPProtocol:         "ip6",
					IPProtocolFallback: true,
					QueryResponse:      []config.QueryResponse{},
				},
			},
		},
		"partial-settings": {
			input: sm.TcpSettings{
				SourceIpAddress: "0.0.0.0",
				Tls:             true,
				IpVersion:       1,
			},
			expected: config.Module{
				Prober:  "tcp",
				Timeout: 0,
				TCP: config.TCPProbe{
					IPProtocol:         "ip4",
					IPProtocolFallback: false,
					QueryResponse:      []config.QueryResponse{},
					TLS:                true,
					SourceIPAddress:    "0.0.0.0",
				},
			},
		},
	}

	for name, testcase := range testcases {
		ctx := context.TODO()
		logger := zerolog.New(io.Discard)
		t.Run(name, func(t *testing.T) {
			actual, err := settingsToModule(ctx, &testcase.input, logger)
			require.NoError(t, err)
			require.Equal(t, &testcase.expected, &actual)
		})
	}
}
