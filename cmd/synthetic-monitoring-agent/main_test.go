package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
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
// was removed; the agent must start and only log a hint.
func TestRemovedProtocolSecretsFeature(t *testing.T) {
	require.NoError(t, run([]string{"sm-agent", "-features", "protocol-secrets", "-version"}, io.Discard))

	features := feature.NewCollection()
	require.NoError(t, features.Set("protocol-secrets"))

	var buf bytes.Buffer
	notifyAboutDeprecatedFeatureFlags(features, zerolog.New(&buf))
	require.Contains(t, buf.String(), "the `protocol-secrets` feature is now permanently enabled")
}
