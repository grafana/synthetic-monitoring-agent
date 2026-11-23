package main

import (
	"bytes"
	"sync"
	"testing"

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
// was removed: the agent must start and warn about that flag only, not
// about k6, which run adds by itself.
func TestDeprecatedFeatureHints(t *testing.T) {
	level := zerolog.GlobalLevel()

	t.Cleanup(func() { zerolog.SetGlobalLevel(level) })

	var out syncBuffer

	// The listen address has no port, so run stops after the startup logs and
	// before it starts the HTTP server or dials the API.
	err := run([]string{"sm-agent", "-features=protocol-secrets", "-api-token=test", "-listen-address=bad"}, &out)
	require.ErrorContains(t, err, "missing port")
	require.Contains(t, out.String(), "the `protocol-secrets` feature is now permanently enabled")
	require.NotContains(t, out.String(), "the `k6` feature")
}

// syncBuffer is safe to read while goroutines that run leaves behind still log.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}
