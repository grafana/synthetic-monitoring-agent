package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClusterAdvertiseAddr(t *testing.T) {
	testcases := map[string]struct {
		explicit    string
		expected    string
		expectedErr string
	}{
		"ipv4 with port":        {explicit: "10.0.0.1:9000", expected: "10.0.0.1:9000"},
		"ipv4 without port":     {explicit: "10.0.0.1", expected: "10.0.0.1:7946"},
		"host with port":        {explicit: "sm-agent-0.sm:9000", expected: "sm-agent-0.sm:9000"},
		"host without port":     {explicit: "sm-agent-0.sm", expected: "sm-agent-0.sm:7946"},
		"ipv6 with port":        {explicit: "[::1]:9000", expected: "[::1]:9000"},
		"ipv6 without port":     {explicit: "::1", expected: "[::1]:7946"},
		"ipv6 brackets no port": {explicit: "[::1]", expected: "[::1]:7946"},
		"empty port":            {explicit: "10.0.0.1:", expectedErr: "invalid port"},
		"zero port":             {explicit: "10.0.0.1:0", expectedErr: "invalid port"},
		"non-numeric port":      {explicit: "10.0.0.1:abc", expectedErr: "invalid port"},
		"service name port":     {explicit: "10.0.0.1:http", expectedErr: "invalid port"},
		"port out of range":     {explicit: "10.0.0.1:65536", expectedErr: "invalid port"},
		"ipv6 empty port":       {explicit: "[::1]:", expectedErr: "invalid port"},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := clusterAdvertiseAddr(tc.explicit, nil, 7946)
			if tc.expectedErr != "" {
				require.ErrorContains(t, err, tc.expectedErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}
