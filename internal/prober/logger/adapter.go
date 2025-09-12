package logger

import (
	"context"
	"log/slog"
	"time"
)

// ToSlog creates a new *slog.Logger that adapts the given Logger to the slog interface
func ToSlog(logger Logger) *slog.Logger {
	return slog.New(slogHandler{logger: logger})
}

// slogHandler is an adapter that converts Logger to slog.Handler
type slogHandler struct {
	logger Logger
}

func (h slogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h slogHandler) Handle(ctx context.Context, r slog.Record) error {
	// Convert slog.Record to key-value pairs for logger.Logger
	attrs := make([]any, 0)
	attrs = append(attrs, "level", r.Level.String())
	attrs = append(attrs, "msg", r.Message)
	attrs = append(attrs, "time", r.Time.Format(time.RFC3339))

	r.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr.Key, attr.Value.String())
		return true
	})

	return h.logger.Log(attrs...)
}

func (h slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// This method is required by the slog.Handler interface but we can return the handler itself
	// since our Logger interface doesn't support structured logging with persistent attributes.
	// In a more complete implementation, we would create a new handler that combines
	// these attributes with any attributes added in future logging calls.
	return h
}

func (h slogHandler) WithGroup(name string) slog.Handler {
	// This method is required by the slog.Handler interface but we can return the handler itself
	// since our Logger interface doesn't support attribute grouping.
	// In a more complete implementation, we would track the group name and prefix attribute keys.
	return h
}
