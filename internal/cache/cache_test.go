package cache

import (
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestDurable(t *testing.T) {
	logger := zerolog.New(io.Discard)

	local, err := NewLocal(LocalConfig{MaxCapacity: 10, Logger: logger})
	require.NoError(t, err)

	testcases := map[string]struct {
		cache Cache
		want  bool
	}{
		"noop":      {cache: NewNoop(logger)},
		"local":     {cache: local},
		"memcached": {cache: &MemcachedClient{}, want: true},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.cache.Durable())
		})
	}
}
