package metamonitoring

import (
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	logproto "github.com/grafana/loki/pkg/push"
)

// EventLogger provides a convenient API for emitting structured event logs
// to the metamonitoring log buffer.
type EventLogger struct {
	buf    LogBuffer
	labels atomic.Value // string
}

func NewEventLogger(buf LogBuffer, labels string) *EventLogger {
	e := &EventLogger{buf: buf}
	e.labels.Store(labels)
	return e
}

// SetLabels atomically updates the stream labels.
func (e *EventLogger) SetLabels(labels string) {
	e.labels.Store(labels)
}

// Log emits a structured event log entry. Fields are key-value pairs.
func (e *EventLogger) Log(msg string, fields ...string) {
	var line strings.Builder
	line.WriteString("msg=")
	line.WriteString(strconv.Quote(msg))

	for i := 0; i+1 < len(fields); i += 2 {
		line.WriteByte(' ')
		line.WriteString(fields[i])
		line.WriteByte('=')
		line.WriteString(strconv.Quote(fields[i+1]))
	}

	e.buf.Append(logproto.Stream{
		Labels: e.labels.Load().(string),
		Entries: []logproto.Entry{{
			Timestamp: time.Now(),
			Line:      line.String(),
		}},
	})
}
