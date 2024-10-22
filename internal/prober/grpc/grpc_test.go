package grpc

import (
	"context"
	"io"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	promcfg "github.com/prometheus/common/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	name := Prober.Name(Prober{})
	require.Equal(t, name, "grpc")
}

func TestNewProber(t *testing.T) {
	testcases := map[string]struct {
		input       model.Check
		expected    Prober
		ExpectError bool
	}{
		"default": {
			input: model.Check{
				Check: sm.Check{
					Target: "www.grafana.com",
					Settings: sm.CheckSettings{
						Grpc: &sm.GrpcSettings{},
					},
				},
			},
			expected: Prober{
				config.Module{
					Prober:  "grpc",
					Timeout: 0,
					GRPC: config.GRPCProbe{
						PreferredIPProtocol: "ip6",
						IPProtocolFallback:  true,
					},
				},
			},
		},
		"no-settings": {
			input: model.Check{
				Check: sm.Check{
					Target: "www.grafana.com",
					Settings: sm.CheckSettings{
						Grpc: nil,
					},
				},
			},
			ExpectError: true,
		},
	}

	ctx := context.Background()
	logger := zerolog.New(io.Discard)

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := NewProber(ctx, tc.input, logger)

			require.Equal(t, &tc.expected, &actual)
			if tc.ExpectError {
				require.Error(t, err, "unsupported check")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSettingsToModule(t *testing.T) {
	testcases := map[string]struct {
		input    sm.GrpcSettings
		expected config.Module
	}{
		"default": {
			input: sm.GrpcSettings{},
			expected: config.Module{
				Prober:  "grpc",
				Timeout: 0,
				GRPC: config.GRPCProbe{
					PreferredIPProtocol: "ip6",
					IPProtocolFallback:  true,
				},
			},
		},
		"custom svc": {
			input: sm.GrpcSettings{
				Service: "customSvc",
			},
			expected: config.Module{
				Prober:  "grpc",
				Timeout: 0,
				GRPC: config.GRPCProbe{
					Service:             "customSvc",
					PreferredIPProtocol: "ip6",
					IPProtocolFallback:  true,
				},
			},
		},
		"ipv4": {
			input: sm.GrpcSettings{
				IpVersion: sm.IpVersion_V4,
			},
			expected: config.Module{
				Prober:  "grpc",
				Timeout: 0,
				GRPC: config.GRPCProbe{
					PreferredIPProtocol: "ip4",
					IPProtocolFallback:  false,
				},
			},
		},
		"tls": {
			input: sm.GrpcSettings{
				Tls: true,
				TlsConfig: &sm.TLSConfig{
					InsecureSkipVerify: true,
					ServerName:         "grafana.com",
				},
			},
			expected: config.Module{
				Prober:  "grpc",
				Timeout: 0,
				GRPC: config.GRPCProbe{
					PreferredIPProtocol: "ip6",
					IPProtocolFallback:  true,
					TLS:                 true,
					TLSConfig: promcfg.TLSConfig{
						InsecureSkipVerify: true,
						ServerName:         "grafana.com",
					},
				},
			},
		},
	}

	ctx := context.Background()
	logger := zerolog.New(io.Discard)

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := settingsToModule(ctx, &tc.input, logger)
			require.NoError(t, err)
			require.Equal(t, &tc.expected, &actual)
		})
	}
}
