package main

import (
	"bytes"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/rs/zerolog"
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

// Deployments may still pass -features protocol-secrets after the flag
// was removed: the hint must be a warning, so it shows at the default log
// level, and must skip k6, which run enables by default.
func TestDeprecatedFeatureHints(t *testing.T) {
	features := feature.NewCollection()
	require.NoError(t, features.Set("k6,protocol-secrets"))

	var out bytes.Buffer

	notifyAboutDeprecatedFeatureFlags(features, zerolog.New(&out))

	require.Contains(t, out.String(), `"level":"warn"`)
	require.Contains(t, out.String(), "the `protocol-secrets` feature is now permanently enabled")
	require.NotContains(t, out.String(), "the `k6` feature")
}
