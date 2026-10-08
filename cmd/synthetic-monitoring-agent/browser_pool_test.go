package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateBrowserPoolConfig(t *testing.T) {
	testcases := map[string]struct {
		input   browserPoolConfig
		wantErr bool
	}{
		"disabled":                {},
		"enabled with addresses":  {input: browserPoolConfig{Enabled: true, Addresses: StringList{"dns+pool:8080"}}},
		"enabled without address": {input: browserPoolConfig{Enabled: true}, wantErr: true},
		"addresses without enabled": {
			input:   browserPoolConfig{Addresses: StringList{"dns+pool:8080"}},
			wantErr: true,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			err := validateBrowserPoolConfig(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}
