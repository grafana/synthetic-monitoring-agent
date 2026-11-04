package k6runner

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPDebug(t *testing.T) {
	var nilDebug *HTTPDebug
	require.Equal(t, HTTPDebugOff, nilDebug.Get())
	require.Empty(t, nilDebug.args())

	var d HTTPDebug
	require.Equal(t, HTTPDebugOff, d.Get())

	require.NoError(t, d.Set(HTTPDebugFull))
	require.Equal(t, HTTPDebugFull, d.Get())

	require.Error(t, d.Set("loud"))
	require.Equal(t, HTTPDebugFull, d.Get(), "an invalid mode must not change the current one")

	require.NoError(t, d.Set(""))
	require.Equal(t, HTTPDebugOff, d.Get())
}
