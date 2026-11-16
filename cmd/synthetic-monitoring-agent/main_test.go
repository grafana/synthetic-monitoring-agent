package main

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
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

func TestSetupCacheKind(t *testing.T) {
	logger := zerolog.Nop()

	testcases := map[string]struct {
		kind     cache.Kind
		servers  []string
		capacity int
		want     cache.Kind
		wantErr  bool
	}{
		"auto without servers is local":       {kind: cache.KindAuto, capacity: 10, want: cache.KindLocal},
		"auto with servers is memcached":      {kind: cache.KindAuto, servers: []string{"127.0.0.1:11211"}, capacity: 10, want: cache.KindMemcached},
		"local":                               {kind: cache.KindLocal, capacity: 10, want: cache.KindLocal},
		"noop":                                {kind: cache.KindNoop, capacity: 10, want: cache.KindNoop},
		"memcached without servers is local":  {kind: cache.KindMemcached, capacity: 10, want: cache.KindLocal},
		"memcached with a bad address errors": {kind: cache.KindMemcached, servers: []string{"no-port"}, capacity: 10, wantErr: true},
		"unknown kind is local":               {kind: cache.Kind("bogus"), capacity: 10, want: cache.KindLocal},
		"local that cannot be built is noop":  {kind: cache.KindLocal, capacity: 0, want: cache.KindNoop},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			// The memcached client resolves IP literals without dialing, so no
			// server is needed to observe the kind.
			_, kind, err := setupCache(t.Context(), tc.kind, tc.servers, tc.capacity, time.Minute, &logger)

			if tc.wantErr {
				require.Error(t, err)
				require.Equal(t, cache.Kind(""), kind)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, kind)
		})
	}
}

func TestCacheScrapersFlagRegistered(t *testing.T) {
	require.NoError(t, run([]string{"agent", "-cache-scrapers", "-version"}, io.Discard))
}
