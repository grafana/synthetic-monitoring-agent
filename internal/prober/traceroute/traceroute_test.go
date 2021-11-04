package traceroute

import (
	"io"
	"testing"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	name := Prober.Name(Prober{})
	require.Equal(t, name, "traceroute")
}

func TestNewProber(t *testing.T) {
	logger := zerolog.New(io.Discard)
	testcases := map[string]struct {
		input       sm.Check
		expected    Prober
		ExpectError bool
	}{
		"default": {
			input: sm.Check{
				Target: "www.grafana.com",
				Settings: sm.CheckSettings{
					Traceroute: &sm.TracerouteSettings{},
				},
			},
			expected: Prober{
				config: Module{
					count:          5,
					timeout:        30 * time.Second,
					hopTimeout:     500 * time.Millisecond,
					interval:       time.Nanosecond,
					hopSleep:       time.Nanosecond,
					maxHops:        64,
					maxUnknownHops: 15,
					ptrLookup:      false,
					ringBufferSize: 50,
					srcAddr:        "",
				},
				logger: logger,
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

	for name, testcase := range testcases {
		logger := zerolog.New(io.Discard)
		t.Run(name, func(t *testing.T) {
			actual, err := NewProber(testcase.input, logger)
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
		input    sm.TracerouteSettings
		expected Module
	}{
		"default": {
			input: sm.TracerouteSettings{},
			expected: Module{
				count:          5,
				timeout:        30 * time.Second,
				hopTimeout:     500 * time.Millisecond,
				interval:       time.Nanosecond,
				hopSleep:       time.Nanosecond,
				maxHops:        64,
				maxUnknownHops: 15,
				ptrLookup:      false,
				ringBufferSize: 50,
				srcAddr:        "",
			},
		},
		"partial-settings": {
			input: sm.TracerouteSettings{
				HopTimeout:     100,
				MaxUnknownHops: 2,
				PtrLookup:      true,
			},
			expected: Module{
				count:          5,
				timeout:        30 * time.Second,
				hopTimeout:     100,
				interval:       time.Nanosecond,
				hopSleep:       time.Nanosecond,
				maxHops:        64,
				maxUnknownHops: 2,
				ptrLookup:      true,
				ringBufferSize: 50,
				srcAddr:        "",
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
