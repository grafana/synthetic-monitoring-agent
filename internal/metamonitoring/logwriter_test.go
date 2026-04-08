package metamonitoring

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestLogShipper(t *testing.T) {
	t.Run("captures logs at min level", func(t *testing.T) {
		buf := NewRingBuffer(100)
		shipper := NewLogShipper(buf, `{source="test"}`, zerolog.WarnLevel)

		_, err := shipper.WriteLevel(zerolog.WarnLevel, []byte(`{"level":"warn","message":"something bad"}`))
		require.NoError(t, err)

		streams := buf.Drain()
		require.Len(t, streams, 1)
		require.Equal(t, `{source="test"}`, streams[0].Labels)
		require.Len(t, streams[0].Entries, 1)
		require.Contains(t, streams[0].Entries[0].Line, "something bad")
	})

	t.Run("captures logs above min level", func(t *testing.T) {
		buf := NewRingBuffer(100)
		shipper := NewLogShipper(buf, `{source="test"}`, zerolog.WarnLevel)

		_, err := shipper.WriteLevel(zerolog.ErrorLevel, []byte(`{"level":"error","message":"very bad"}`))
		require.NoError(t, err)

		streams := buf.Drain()
		require.Len(t, streams, 1)
	})

	t.Run("filters logs below min level", func(t *testing.T) {
		buf := NewRingBuffer(100)
		shipper := NewLogShipper(buf, `{source="test"}`, zerolog.WarnLevel)

		_, err := shipper.WriteLevel(zerolog.InfoLevel, []byte(`{"level":"info","message":"normal"}`))
		require.NoError(t, err)

		_, err = shipper.WriteLevel(zerolog.DebugLevel, []byte(`{"level":"debug","message":"noisy"}`))
		require.NoError(t, err)

		streams := buf.Drain()
		require.Empty(t, streams)
	})

	t.Run("fallback Write is a no-op", func(t *testing.T) {
		buf := NewRingBuffer(100)
		shipper := NewLogShipper(buf, `{source="test"}`, zerolog.WarnLevel)

		n, err := shipper.Write([]byte("some bytes"))
		require.NoError(t, err)
		require.Equal(t, 10, n)

		streams := buf.Drain()
		require.Empty(t, streams)
	})

	t.Run("works with zerolog MultiLevelWriter", func(t *testing.T) {
		buf := NewRingBuffer(100)
		shipper := NewLogShipper(buf, `{source="test"}`, zerolog.WarnLevel)

		logger := zerolog.New(zerolog.MultiLevelWriter(zerolog.NewTestWriter(t), shipper))

		logger.Info().Msg("should not ship")
		logger.Warn().Msg("should ship")
		logger.Error().Msg("should also ship")

		streams := buf.Drain()
		require.Len(t, streams, 2)
		require.Contains(t, streams[0].Entries[0].Line, "should ship")
		require.Contains(t, streams[1].Entries[0].Line, "should also ship")
	})
}
