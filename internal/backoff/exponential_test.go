package backoff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewBinary(t *testing.T) {
	baseDelay := time.Duration(100)
	maxExp := int64(4)

	b := NewBinary(baseDelay, maxExp)

	require.Equal(t, int64(0), b.c)
	require.Equal(t, baseDelay, b.baseDelay)
	require.Equal(t, maxExp, b.maxExp)
}

func TestBinaryGet(t *testing.T) {
	baseDelay := time.Duration(100)
	maxExp := int64(10)

	b := NewBinary(baseDelay, maxExp)

	for n := int64(1); n <= maxExp; n++ {
		actual := b.Get()

		require.Equal(t, n, b.c)
		require.Equal(t, baseDelay, b.baseDelay)
		require.Equal(t, maxExp, b.maxExp)

		require.GreaterOrEqual(t, actual, time.Duration(0))
		require.LessOrEqual(t, actual, time.Duration(((1<<n)-1)*baseDelay))
		require.Equal(t, time.Duration(0), actual%baseDelay)
	}

	// calling it one more time should not increase the counter

	actual := b.Get()

	require.Equal(t, maxExp, b.c)
	require.Equal(t, baseDelay, b.baseDelay)
	require.Equal(t, maxExp, b.maxExp)

	require.GreaterOrEqual(t, actual, time.Duration(0))
	require.LessOrEqual(t, actual, time.Duration(((1<<maxExp)-1)*baseDelay))
	require.Equal(t, time.Duration(0), actual%baseDelay)
}

func TestBinaryReset(t *testing.T) {
	baseDelay := time.Duration(100)
	maxExp := int64(10)

	b := NewBinary(baseDelay, maxExp)

	_ = b.Get()

	require.Equal(t, int64(1), b.c)

	b.Reset()

	require.Equal(t, int64(0), b.c)
}
