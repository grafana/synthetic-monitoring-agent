package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKindSet(t *testing.T) {
	for _, accepted := range []Kind{KindAuto, KindLocal, KindMemcached, KindNoop, KindRedis} {
		t.Run(string(accepted), func(t *testing.T) {
			var k Kind

			require.NoError(t, k.Set(string(accepted)))
			require.Equal(t, accepted, k)
			require.Equal(t, string(accepted), k.String())
		})
	}

	t.Run("unsupported", func(t *testing.T) {
		k := KindAuto

		require.ErrorIs(t, k.Set("bogus"), ErrUnsupportedCacheKind)
		require.Equal(t, KindAuto, k)
	})
}
