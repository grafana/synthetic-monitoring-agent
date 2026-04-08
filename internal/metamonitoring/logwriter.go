package metamonitoring

import (
	"sync/atomic"
	"time"

	logproto "github.com/grafana/loki/pkg/push"
	"github.com/rs/zerolog"
)

// LogShipper is a zerolog.LevelWriter that captures log events at or above
// a minimum level and appends them to a LogBuffer as operational log streams.
type LogShipper struct {
	buf      LogBuffer
	labels   atomic.Value // string
	minLevel zerolog.Level
}

func NewLogShipper(buf LogBuffer, labels string, minLevel zerolog.Level) *LogShipper {
	s := &LogShipper{
		buf:      buf,
		minLevel: minLevel,
	}
	s.labels.Store(labels)
	return s
}

// SetLabels atomically updates the stream labels. Use this to enrich
// labels with information that becomes available after construction
// (e.g., probe name after registration).
func (w *LogShipper) SetLabels(labels string) {
	w.labels.Store(labels)
}

// Write is the fallback for writers that don't call WriteLevel.
// We skip these since we can't determine the level.
func (w *LogShipper) Write(p []byte) (int, error) {
	return len(p), nil
}

// WriteLevel is called by zerolog's MultiLevelWriter with the event level.
func (w *LogShipper) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	if level < w.minLevel {
		return len(p), nil
	}

	w.buf.Append(logproto.Stream{
		Labels: w.labels.Load().(string),
		Entries: []logproto.Entry{{
			Timestamp: time.Now(),
			Line:      string(p),
		}},
	})

	return len(p), nil
}
