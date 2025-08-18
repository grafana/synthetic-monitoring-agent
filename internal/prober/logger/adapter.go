package logger

import (
	"context"
	"log/slog"

	"github.com/rs/zerolog"
)

// FromZerolog creates a new Logger that adapts zerolog.Logger to our Logger interface
func FromZerolog(zl zerolog.Logger) Logger {
	return zerologAdapter{logger: zl}
}

// ToSlog creates a new *slog.Logger that adapts the given Logger to the slog interface
func ToSlog(logger Logger) *slog.Logger {
	return slog.New(slogHandler{logger: logger})
}

// zerologAdapter adapts zerolog.Logger to our Logger interface
type zerologAdapter struct {
	logger zerolog.Logger
}

func (a zerologAdapter) Log(keyvals ...interface{}) error {
	if len(keyvals) == 0 {
		return nil
	}

	// Handle key-value pairs
	if len(keyvals)%2 == 0 {
		event := a.logger.Info()
		for i := 0; i < len(keyvals); i += 2 {
			key, ok := keyvals[i].(string)
			if !ok {
				continue
			}
			event = event.Interface(key, keyvals[i+1])
		}
		event.Send()
		return nil
	}

	// Handle single message
	if len(keyvals) == 1 {
		if msg, ok := keyvals[0].(string); ok {
			a.logger.Info().Msg(msg)
		}
		return nil
	}

	return nil
}

// slogHandler adapts our Logger interface to slog.Handler
type slogHandler struct {
	logger Logger
}

func (h slogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h slogHandler) Handle(ctx context.Context, record slog.Record) error {
	keyvals := make([]interface{}, 0, record.NumAttrs()*2+2)
	keyvals = append(keyvals, "msg", record.Message)
	keyvals = append(keyvals, "level", record.Level.String())

	record.Attrs(func(attr slog.Attr) bool {
		keyvals = append(keyvals, attr.Key, attr.Value.Any())
		return true
	})

	return h.logger.Log(keyvals...)
}

func (h slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h slogHandler) WithGroup(name string) slog.Handler {
	return h
}
