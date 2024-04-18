package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecret(t *testing.T) {
	testcases := map[string]struct {
		input string
	}{
		"empty":  {input: ""},
		"secret": {input: "secret"},
		"blank":  {input: " "},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var s Secret

			require.NoError(t, s.Set(tc.input))

			require.Equal(t, tc.input, string(s))

			require.Equal(t, "<redacted>", s.String())

			text, err := s.MarshalText()
			require.NoError(t, err)
			require.Equal(t, []byte("<redacted>"), text)

			buf, err := json.Marshal(s)
			require.NoError(t, err)
			require.Equal(t, []byte(`"\u003credacted\u003e"`), buf)
		})
	}
}
