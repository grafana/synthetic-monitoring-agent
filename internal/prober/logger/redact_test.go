package logger

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-kit/log/level"
	"github.com/stretchr/testify/require"
)

type recordingLogger struct {
	keyvals []any
}

func (r *recordingLogger) Log(keyvals ...any) error {
	r.keyvals = keyvals

	return nil
}

type stringer string

func (s stringer) String() string { return string(s) }

func TestRedacting(t *testing.T) {
	redact := strings.NewReplacer("hunter2", "${secrets.password}").Replace

	t.Run("nil redact returns the logger", func(t *testing.T) {
		var rec recordingLogger

		require.Equal(t, Logger(&rec), Redacting(&rec, nil))
	})

	t.Run("every kind of text is redacted", func(t *testing.T) {
		var rec recordingLogger

		err := Redacting(&rec, redact).Log(
			"string", "a hunter2 b",
			"bytes", []byte("hunter2"),
			"error", errors.New("dial hunter2"),
			"stringer", stringer("hunter2"),
			"number", 42,
		)
		require.NoError(t, err)

		require.Equal(t, []any{
			"string", "a ${secrets.password} b",
			"bytes", "${secrets.password}",
			"error", "dial ${secrets.password}",
			"stringer", "${secrets.password}",
			"number", 42,
		}, rec.keyvals)
	})

	t.Run("keys and timestamps are never redacted", func(t *testing.T) {
		var rec recordingLogger

		short := strings.NewReplacer("16", "${secrets.short}").Replace

		err := Redacting(&rec, short).Log(
			"level", "DEBUG",
			"msg", "port 16",
			"time", "2026-11-16T16:20:39Z",
			"ts", "2026-11-16T16:20:39Z",
			"16", "16",
		)
		require.NoError(t, err)

		require.Equal(t, []any{
			"level", "DEBUG",
			"msg", "port ${secrets.short}",
			"time", "2026-11-16T16:20:39Z",
			"ts", "2026-11-16T16:20:39Z",
			"16", "${secrets.short}",
		}, rec.keyvals)
	})

	t.Run("a value without a secret keeps its type", func(t *testing.T) {
		var rec recordingLogger

		err := level.Error(Redacting(&rec, redact)).Log("err", errors.New("plain"))
		require.NoError(t, err)

		require.Equal(t, level.ErrorValue(), rec.keyvals[1])
		require.Implements(t, (*error)(nil), rec.keyvals[3])
	})
}
