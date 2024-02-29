package telemetry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWithJitter(t *testing.T) {
	testCases := []struct {
		d      time.Duration
		minExp time.Duration
		maxExp time.Duration
	}{
		{
			d:      1 * time.Minute,
			minExp: 1 * time.Minute,
			maxExp: 1*time.Minute + jitterUpperBound*time.Second,
		},
		{
			d:      5 * time.Minute,
			minExp: 5 * time.Minute,
			maxExp: 5*time.Minute + jitterUpperBound*time.Second,
		},
		{
			d:      60 * time.Second,
			minExp: 60 * time.Second,
			maxExp: 60*time.Second + jitterUpperBound*time.Second,
		},
		{
			d:      5 * time.Second,
			minExp: 5 * time.Second,
			maxExp: 5*time.Second + jitterUpperBound*time.Second,
		},
		{
			d:      0 * time.Second,
			minExp: 0 * time.Second,
			maxExp: 0*time.Second + jitterUpperBound*time.Second,
		},
	}

	for _, tc := range testCases {
		require.GreaterOrEqual(t, withJitter(tc.d), tc.minExp)
		require.Less(t, withJitter(tc.d), tc.maxExp)
	}
}
