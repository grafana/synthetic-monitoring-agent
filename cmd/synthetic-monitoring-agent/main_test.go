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
