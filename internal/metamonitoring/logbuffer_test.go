package metamonitoring

import (
	"fmt"
	"sync"
	"testing"

	logproto "github.com/grafana/loki/pkg/push"
	"github.com/stretchr/testify/require"
)

func makeStream(label string) logproto.Stream {
	return logproto.Stream{Labels: label}
}

func TestRingBuffer(t *testing.T) {
	t.Run("drain returns appended streams", func(t *testing.T) {
		buf := NewRingBuffer(10)
		buf.Append(makeStream("a"))
		buf.Append(makeStream("b"))

		got := buf.Drain()
		require.Len(t, got, 2)
		require.Equal(t, "a", got[0].Labels)
		require.Equal(t, "b", got[1].Labels)
	})

	t.Run("drain empties the buffer", func(t *testing.T) {
		buf := NewRingBuffer(10)
		buf.Append(makeStream("a"))

		_ = buf.Drain()
		got := buf.Drain()
		require.Empty(t, got)
	})

	t.Run("drops oldest on overflow", func(t *testing.T) {
		buf := NewRingBuffer(3)
		buf.Append(makeStream("a"))
		buf.Append(makeStream("b"))
		buf.Append(makeStream("c"))
		buf.Append(makeStream("d"))

		got := buf.Drain()
		require.Len(t, got, 3)
		require.Equal(t, "b", got[0].Labels)
		require.Equal(t, "c", got[1].Labels)
		require.Equal(t, "d", got[2].Labels)
	})

	t.Run("tracks dropped count", func(t *testing.T) {
		buf := NewRingBuffer(2)
		buf.Append(makeStream("a"))
		buf.Append(makeStream("b"))
		buf.Append(makeStream("c"))
		buf.Append(makeStream("d"))

		require.Equal(t, int64(2), buf.Dropped())
	})

	t.Run("drain does not reset dropped count", func(t *testing.T) {
		buf := NewRingBuffer(1)
		buf.Append(makeStream("a"))
		buf.Append(makeStream("b"))
		_ = buf.Drain()

		require.Equal(t, int64(1), buf.Dropped())
	})

	t.Run("concurrent append and drain", func(t *testing.T) {
		buf := NewRingBuffer(100)
		var wg sync.WaitGroup

		// Concurrent writers.
		for i := range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range 50 {
					buf.Append(makeStream(fmt.Sprintf("%d-%d", i, j)))
				}
			}()
		}

		// Concurrent drainers.
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = buf.Drain()
			}()
		}

		wg.Wait()

		// No panics, no data corruption. Buffer should have <= maxItems.
		remaining := buf.Drain()
		require.LessOrEqual(t, len(remaining), 100)
	})

	t.Run("empty drain returns empty slice", func(t *testing.T) {
		buf := NewRingBuffer(10)
		got := buf.Drain()
		require.NotNil(t, got)
		require.Empty(t, got)
	})
}
