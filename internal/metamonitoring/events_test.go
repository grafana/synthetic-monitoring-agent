package metamonitoring

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventLogger(t *testing.T) {
	t.Run("emits logfmt event", func(t *testing.T) {
		buf := NewRingBuffer(100)
		el := NewEventLogger(buf, `{source="test",log_type="event"}`)

		el.Log("check_added", "check_id", "123", "target", "example.com")

		streams := buf.Drain()
		require.Len(t, streams, 1)
		require.Equal(t, `{source="test",log_type="event"}`, streams[0].Labels)
		require.Len(t, streams[0].Entries, 1)
		require.Equal(t, `msg="check_added" check_id="123" target="example.com"`, streams[0].Entries[0].Line)
	})

	t.Run("emits event with no fields", func(t *testing.T) {
		buf := NewRingBuffer(100)
		el := NewEventLogger(buf, `{source="test"}`)

		el.Log("probe_registered")

		streams := buf.Drain()
		require.Len(t, streams, 1)
		require.Equal(t, `msg="probe_registered"`, streams[0].Entries[0].Line)
	})

	t.Run("odd number of fields drops last", func(t *testing.T) {
		buf := NewRingBuffer(100)
		el := NewEventLogger(buf, `{source="test"}`)

		el.Log("event", "key1", "val1", "orphan")

		streams := buf.Drain()
		require.Len(t, streams, 1)
		require.Equal(t, `msg="event" key1="val1"`, streams[0].Entries[0].Line)
	})

	t.Run("SetLabels updates labels atomically", func(t *testing.T) {
		buf := NewRingBuffer(100)
		el := NewEventLogger(buf, `{source="test"}`)

		el.Log("before")
		el.SetLabels(`{source="test",probe="clamps"}`)
		el.Log("after")

		streams := buf.Drain()
		require.Len(t, streams, 2)
		require.Equal(t, `{source="test"}`, streams[0].Labels)
		require.Equal(t, `{source="test",probe="clamps"}`, streams[1].Labels)
	})

	t.Run("quotes special characters in values", func(t *testing.T) {
		buf := NewRingBuffer(100)
		el := NewEventLogger(buf, `{source="test"}`)

		el.Log("error", "msg", `connection "reset"`)

		streams := buf.Drain()
		require.Len(t, streams, 1)
		require.Contains(t, streams[0].Entries[0].Line, `msg="error"`)
		require.Contains(t, streams[0].Entries[0].Line, `msg="connection \"reset\""`)
	})
}
