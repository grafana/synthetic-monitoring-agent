package main

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/cache"
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

func TestSetupCacheRejectsRedis(t *testing.T) {
	logger := zerolog.Nop()

	_, err := setupCache(t.Context(), cache.KindRedis, nil, 10, time.Minute, &logger)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported for the tenants cache")
}
