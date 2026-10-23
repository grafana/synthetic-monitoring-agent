package main

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestResolveClusterNodeName(t *testing.T) {
	t.Run("uses configured name", func(t *testing.T) {
		hostnameCalled := false
		name, err := resolveClusterNodeName("sm-agent-2", func() (string, error) {
			hostnameCalled = true
			return "sm-agent-0", nil
		})

		require.NoError(t, err)
		require.Equal(t, "sm-agent-2", name)
		require.False(t, hostnameCalled)
	})

	t.Run("uses hostname when name is not configured", func(t *testing.T) {
		name, err := resolveClusterNodeName("", func() (string, error) {
			return "sm-agent-0", nil
		})

		require.NoError(t, err)
		require.Equal(t, "sm-agent-0", name)
	})

	t.Run("generates unique names when hostname resolution fails", func(t *testing.T) {
		hostnameErr := errors.New("hostname unavailable")
		resolve := func() (string, error) {
			return "", hostnameErr
		}

		first, err := resolveClusterNodeName("", resolve)
		require.ErrorIs(t, err, hostnameErr)

		_, parseErr := uuid.Parse(first)
		require.NoError(t, parseErr)

		second, err := resolveClusterNodeName("", resolve)
		require.ErrorIs(t, err, hostnameErr)

		_, parseErr = uuid.Parse(second)
		require.NoError(t, parseErr)
		require.NotEqual(t, first, second)
	})

	t.Run("generates a name when hostname is empty", func(t *testing.T) {
		name, err := resolveClusterNodeName("", func() (string, error) {
			return "", nil
		})

		require.EqualError(t, err, "hostname is empty")

		_, parseErr := uuid.Parse(name)
		require.NoError(t, parseErr)
	})
}

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
