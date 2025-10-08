package agenttelemetry

import (
	"time"

	"github.com/rs/zerolog"
)

// Hook implements zerolog.Hook interface to capture agent logs
type Hook struct {
	collector *Collector
}

// NewHook creates a new zerolog hook for agent telemetry
func NewHook(collector *Collector) *Hook {
	return &Hook{
		collector: collector,
	}
}

// Run implements zerolog.Hook interface
func (h *Hook) Run(e *zerolog.Event, level zerolog.Level, message string) {
	// Extract fields from the event
	fields := make(map[string]interface{})

	// Add basic fields
	fields["level"] = level.String()
	fields["message"] = message

	// Try to extract additional fields from the event
	// This is a simplified approach - zerolog doesn't provide easy access to all fields
	// but we can add some common ones
	if e != nil {
		// Add timestamp
		fields["timestamp"] = time.Now().Unix()

		// Add any other fields that might be useful
		// Note: zerolog doesn't expose all fields easily, so this is limited
	}

	// Add the log entry to the collector
	h.collector.AddLog(level.String(), message, fields)
}
