// Package logger provides a minimal implementation of slog.Handler that bridges to zerolog.
// This implementation is inspired by and based on the samber/slog-zerolog library,
package logger

import (
	"context"
	"log/slog"

	"github.com/rs/zerolog"
)

// NewSlogFromZerolog creates a new *slog.Logger that uses zerolog.Logger as the backend
// This is a minimal implementation that only includes what we actually use
func NewSlogFromZerolog(zlogger zerolog.Logger) *slog.Logger {
	return slog.New(&zerologHandler{logger: zlogger})
}

// zerologHandler implements slog.Handler by bridging to zerolog
type zerologHandler struct {
	logger zerolog.Logger
}

// Enabled determines if the log level is enabled
func (h *zerologHandler) Enabled(ctx context.Context, level slog.Level) bool {
	// Map slog levels to zerolog levels
	switch level {
	case slog.LevelDebug:
		return h.logger.GetLevel() <= zerolog.DebugLevel
	case slog.LevelInfo:
		return h.logger.GetLevel() <= zerolog.InfoLevel
	case slog.LevelWarn:
		return h.logger.GetLevel() <= zerolog.WarnLevel
	case slog.LevelError:
		return h.logger.GetLevel() <= zerolog.ErrorLevel
	default:
		return false
	}
}

// Handle logs the record using zerolog
func (h *zerologHandler) Handle(ctx context.Context, record slog.Record) error {
	// Map slog levels to zerolog levels
	var lvl zerolog.Level
	switch record.Level {
	case slog.LevelDebug:
		lvl = zerolog.DebugLevel
	case slog.LevelInfo:
		lvl = zerolog.InfoLevel
	case slog.LevelWarn:
		lvl = zerolog.WarnLevel
	case slog.LevelError:
		lvl = zerolog.ErrorLevel
	default:
		lvl = zerolog.NoLevel
	}

	// Create a zerolog event and add attributes
	event := h.logger.WithLevel(lvl).Timestamp() //nolint:zerologlint
	record.Attrs(func(attr slog.Attr) bool {
		event = event.Interface(attr.Key, attr.Value.Any())
		return true
	})
	event.Msg(record.Message)
	return nil
}

// WithAttrs returns a new handler with additional attributes
func (h *zerologHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Convert attributes to a map for zerolog
	fields := make(map[string]interface{}, len(attrs))
	for _, attr := range attrs {
		fields[attr.Key] = attr.Value.Any()
	}

	// Clone the logger and add attributes
	newLogger := h.logger.With().Fields(fields).Logger()
	return &zerologHandler{logger: newLogger}
}

// WithGroup returns a new handler with a group name
func (h *zerologHandler) WithGroup(name string) slog.Handler {
	// For simplicity, we'll just add the group as a field
	// This is a minimal implementation - we could make it more sophisticated if needed
	newLogger := h.logger.With().Str("group", name).Logger()
	return &zerologHandler{logger: newLogger}
}
