package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringList(t *testing.T) {
	{
		var sl StringList
		require.Equal(t, "", sl.String())
	}

	{
		var sl StringList
		require.NoError(t, sl.Set("a"))
		require.Equal(t, []string{"a"}, []string(sl))
		require.Equal(t, "a", sl.String())
	}

	{
		var sl StringList
		require.NoError(t, sl.Set("a,b"))
		require.Equal(t, []string{"a", "b"}, []string(sl))
		require.Equal(t, "a, b", sl.String())
	}

	{
		var sl StringList
		require.NoError(t, sl.Set("a,b,c"))
		require.Equal(t, []string{"a", "b", "c"}, []string(sl))
		require.Equal(t, "a, b, c", sl.String())
	}

	{
		var sl StringList
		require.NoError(t, sl.Set("a, b"))
		require.Equal(t, []string{"a", "b"}, []string(sl))
		require.Equal(t, "a, b", sl.String())
	}

	{
		var sl StringList
		require.NoError(t, sl.Set("  a,    b    "))
		require.Equal(t, []string{"a", "b"}, []string(sl))
		require.Equal(t, "a, b", sl.String())
	}
}
