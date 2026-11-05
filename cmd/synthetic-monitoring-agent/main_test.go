package main

import (
	"bytes"
	"fmt"
	"io"
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

func TestSelectScraperCache(t *testing.T) {
	agentCache := cache.NewNoop(zerolog.Nop())

	testcases := map[string]struct {
		enabled   bool
		agentKind cache.Kind
		wantAgent bool // the agent's cache is returned; otherwise nil, meaning disabled
		wantWarn  string
	}{
		"disabled": {
			agentKind: cache.KindMemcached,
		},
		"enabled with memcached": {
			enabled:   true,
			agentKind: cache.KindMemcached,
			wantAgent: true,
		},
		"enabled with local warns about restarts": {
			enabled:   true,
			agentKind: cache.KindLocal,
			wantAgent: true,
			wantWarn:  "in-process",
		},
		"enabled with noop warns and disables": {
			enabled:   true,
			agentKind: cache.KindNoop,
			wantWarn:  "no-op",
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer

			got := selectScraperCache(tc.enabled, agentCache, tc.agentKind, zerolog.New(&buf))

			if tc.wantAgent {
				require.Same(t, agentCache, got)
			} else {
				require.Nil(t, got)
			}

			if tc.wantWarn == "" {
				require.NotContains(t, buf.String(), `"level":"warn"`)

				if tc.enabled {
					require.Contains(t, buf.String(), "scraper cache enabled")
					require.Contains(t, buf.String(), fmt.Sprintf(`"backend":%q`, tc.agentKind))
				}
			} else {
				require.Contains(t, buf.String(), `"level":"warn"`)
				require.Contains(t, buf.String(), tc.wantWarn)
			}
		})
	}
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
