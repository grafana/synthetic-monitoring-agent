package traceroute

import (
	"testing"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/require"
)

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
				ptrLookup:      true,
				ringBufferSize: 50,
				srcAddr:        "",
			},
		},
		"partial-settings": {
			input: sm.TracerouteSettings{
				HopTimeout:     100,
				MaxUnknownHops: 2,
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
