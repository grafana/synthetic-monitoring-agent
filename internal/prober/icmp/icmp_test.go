package icmp

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	bbeprober "github.com/prometheus/blackbox_exporter/prober"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	name := Prober.Name(Prober{})
	require.Equal(t, name, "ping")
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
					Ping: &sm.PingSettings{},
				},
			},
			expected: Prober{
				config: Module{
					Prober:      "ping",
					Timeout:     0,
					PacketCount: 1,
					Privileged:  isPrivilegedRequired(),
					ICMP: config.ICMPProbe{
						IPProtocol:         "ip6",
						IPProtocolFallback: true,
					},
				},
			},
			ExpectError: false,
		},
		"no-settings": {
			input: sm.Check{
				Target: "www.grafana.com",
				Settings: sm.CheckSettings{
					Http: nil,
				},
			},
			expected:    Prober{},
			ExpectError: true,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := NewProber(testcase.input)
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
		input    sm.PingSettings
		expected Module
	}{
		"default": {
			input: sm.PingSettings{},
			expected: Module{
				Prober:      "ping",
				Timeout:     0,
				PacketCount: 1,
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
			expected: Module{
				Prober:      "ping",
				Timeout:     0,
				PacketCount: 1,
				ICMP: config.ICMPProbe{
					IPProtocol:         "ip4",
					IPProtocolFallback: false,
					SourceIPAddress:    "0.0.0.0",
				},
			},
		},
		"count 1": {
			input: sm.PingSettings{
				IpVersion:   1,
				PacketCount: 1,
			},
			expected: Module{
				Prober:      "ping",
				Timeout:     0,
				PacketCount: 1,
				ICMP: config.ICMPProbe{
					IPProtocol:         "ip4",
					IPProtocolFallback: false,
				},
			},
		},
		"count 2": {
			input: sm.PingSettings{
				IpVersion:   1,
				PacketCount: 2,
			},
			expected: Module{
				Prober:      "ping",
				Timeout:     0,
				PacketCount: 2,
				ICMP: config.ICMPProbe{
					IPProtocol:         "ip4",
					IPProtocolFallback: false,
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

func TestProber(t *testing.T) {
	deadline, ok := t.Deadline()
	if !ok {
		deadline = time.Now().Add(1 * time.Second)
	}

	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	prober, err := NewProber(sm.Check{
		Target:  "127.0.0.1",
		Timeout: 1000,
		Settings: sm.CheckSettings{
			Ping: &sm.PingSettings{
				IpVersion:   sm.IpVersion_V4,
				PacketCount: 1,
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, prober)

	registry := prometheus.NewRegistry()
	require.NotNil(t, registry)

	logger := log.NewLogfmtLogger(os.Stdout)
	require.NotNil(t, logger)

	success := prober.Probe(ctx, "127.0.0.1", registry, logger)
	require.True(t, success)
}

func TestBBEProber(t *testing.T) {
	deadline, ok := t.Deadline()
	if !ok {
		deadline = time.Now().Add(1 * time.Second)
	}

	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	target := "127.0.0.1"

	registry := prometheus.NewRegistry()
	require.NotNil(t, registry)

	logger := log.NewLogfmtLogger(os.Stdout)
	require.NotNil(t, logger)

	module := config.Module{
		Prober:  "test",
		Timeout: 100 * time.Millisecond,
		ICMP: config.ICMPProbe{
			IPProtocol:         "ip4",
			IPProtocolFallback: false,
		},
	}

	success := bbeprober.ProbeICMP(ctx, target, module, registry, logger)
	require.True(t, success)
}
