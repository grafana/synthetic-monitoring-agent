package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateCIDRs(t *testing.T) {
	testcases := map[string]struct {
		input   string
		wantErr bool
	}{
		"empty":           {input: ""},
		"single":          {input: "10.0.0.0/8"},
		"multiple":        {input: "10.0.0.0/8,127.0.0.0/8"},
		"ipv6":            {input: "10.0.0.0/8,fd00::/8"},
		"whitespace":      {input: "  ", wantErr: true},
		"spaces":          {input: "10.0.0.0/8, 127.0.0.0/8", wantErr: true},
		"invalid":         {input: "10.0.0.0/8,not-a-cidr", wantErr: true},
		"empty entry":     {input: "10.0.0.0/8,,127.0.0.0/8", wantErr: true},
		"space separated": {input: "10.0.0.0/8 127.0.0.0/8", wantErr: true},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			err := validateCIDRs(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}
