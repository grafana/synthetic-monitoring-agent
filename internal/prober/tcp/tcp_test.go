package tcp

import (
	"context"
	"io"
	"testing"
	"time"

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
		input       sm.Check
		expected    Prober
		ExpectError bool
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
			ExpectError: false,
		},
		"no-settings": {
			input: sm.Check{
				Target: "www.grafana.com",
				Settings: sm.CheckSettings{
					Tcp: nil,
				},
			},
			expected:    Prober{},
			ExpectError: true,
		},
	}

	ctx := testCtx(context.Background(), t)

	for name, testcase := range testcases {
		logger := zerolog.New(io.Discard)
		t.Run(name, func(t *testing.T) {
			actual, err := NewProber(ctx, testcase.input, logger)
			require.Equal(t, &testcase.expected, &actual)
			if testcase.ExpectError {
				require.Error(t, err, "unsupported check")
			} else {
				require.NoError(t, err)
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

	ctx := testCtx(context.Background(), t)

	for name, testcase := range testcases {
		logger := zerolog.New(io.Discard)
		t.Run(name, func(t *testing.T) {
			actual, err := settingsToModule(ctx, &testcase.input, logger)
			require.NoError(t, err)
			require.Equal(t, &testcase.expected, &actual)
		})
	}
}

func testCtx(ctx context.Context, t *testing.T) context.Context {
	if deadline, ok := t.Deadline(); ok {
		ctx, cancel := context.WithDeadline(ctx, deadline)
		t.Cleanup(cancel)
		return ctx
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	t.Cleanup(cancel)

	return ctx
}
