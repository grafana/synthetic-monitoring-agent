package main

import (
	"context"
	"log/slog"

	"go.uber.org/zap/zapcore"
)

// SlogCore implements zapcore.Core by routing entries to a slog.Handler
type SlogCore struct {
	handler slog.Handler
	level   zapcore.LevelEnabler
	fields  []slog.Attr
}

func NewSlogCore(handler slog.Handler, level zapcore.LevelEnabler) zapcore.Core {
	return &SlogCore{
		handler: handler,
		level:   level,
	}
}

func (s *SlogCore) Enabled(lvl zapcore.Level) bool {
	return s.level.Enabled(lvl)
}

// With returns a cloned core containing added structured context fields
func (s *SlogCore) With(fields []zapcore.Field) zapcore.Core {
	clone := &SlogCore{
		handler: s.handler,
		level:   s.level,
		fields:  make([]slog.Attr, len(s.fields)),
	}
	copy(clone.fields, s.fields)

	// Convert Zap fields to slog Attrs
	for _, f := range fields {
		clone.fields = append(clone.fields, s.convertField(f))
	}
	return clone
}

// Check registers this core with a zapcore.CheckedEntry if the level is enabled
func (s *SlogCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if s.Enabled(ent.Level) {
		return ce.AddCore(ent, s)
	}
	return ce
}

// Write formats and executes the log record into the slog handler
func (s *SlogCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	ctx := context.Background()
	lvl := s.convertLevel(ent.Level)

	if !s.handler.Enabled(ctx, lvl) {
		return nil
	}

	// Prepare the slog Record
	record := slog.NewRecord(ent.Time, lvl, ent.Message, 0)

	// Add context fields pre-attached via Core.With()
	record.AddAttrs(s.fields...)

	// Add fields specific to this log call
	for _, f := range fields {
		record.AddAttrs(s.convertField(f))
	}

	// Optional: Include caller details if Zap configuration asks for it
	if ent.Caller.Defined {
		record.AddAttrs(slog.String("caller", ent.Caller.String()))
	}

	return s.handler.Handle(ctx, record)
}

func (s *SlogCore) Sync() error {
	return nil
}

// Helper: Convert Zap logging levels to slog levels
func (s *SlogCore) convertLevel(lvl zapcore.Level) slog.Level {
	switch lvl {
	case zapcore.DebugLevel:
		return slog.LevelDebug
	case zapcore.InfoLevel:
		return slog.LevelInfo
	case zapcore.WarnLevel:
		return slog.LevelWarn
	case zapcore.ErrorLevel, zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Helper: Safely map a Zap Field definition to a slog Attribute
func (s *SlogCore) convertField(f zapcore.Field) slog.Attr {
	// For maximum fidelity, you can switch on f.Type.
	// This fallback extracts via Zap's internal formatting mechanisms:
	switch f.Type {
	case zapcore.StringType:
		return slog.String(f.Key, f.String)
	case zapcore.Int64Type:
		return slog.Int64(f.Key, f.Integer)
	case zapcore.BoolType:
		return slog.Bool(f.Key, f.Integer == 1)
	default:
		// Fallback for objects, arrays, and complex structs
		encoder := zapcore.NewMapObjectEncoder()
		f.AddTo(encoder)
		if val, exists := encoder.Fields[f.Key]; exists {
			return slog.Any(f.Key, val)
		}
		return slog.Any(f.Key, f.Interface)
	}
}
