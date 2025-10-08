package agenttelemetry

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"

	logproto "github.com/grafana/loki/pkg/push"
)

// Collector captures agent logs and metrics for telemetry
type Collector struct {
	mu         sync.RWMutex
	enabled    bool
	logs       []logproto.Entry
	metrics    []prompb.TimeSeries
	maxLogs    int
	maxMetrics int
	logger     zerolog.Logger
	probeName  string
	region     string
}

// NewCollector creates a new agent telemetry collector
func NewCollector(logger zerolog.Logger, maxLogs, maxMetrics int) *Collector {
	return &Collector{
		logs:       make([]logproto.Entry, 0, maxLogs),
		metrics:    make([]prompb.TimeSeries, 0, maxMetrics),
		maxLogs:    maxLogs,
		maxMetrics: maxMetrics,
		logger:     logger,
	}
}

// SetProbeInfo sets the probe name and region for labeling
func (c *Collector) SetProbeInfo(probeName, region string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.probeName = probeName
	c.region = region
}

// Enable enables the collector
func (c *Collector) Enable() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = true
}

// Disable disables the collector
func (c *Collector) Disable() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = false
}

// AddLog adds a log entry to the collector
func (c *Collector) AddLog(level, message string, fields map[string]interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.enabled {
		return
	}

	// Create log entry
	entry := logproto.Entry{
		Timestamp: time.Now(),
		Line:      c.formatLogLine(level, message, fields),
	}

	// Add to logs, maintaining max size
	if len(c.logs) >= c.maxLogs {
		c.logs = c.logs[1:] // Remove oldest
	}
	c.logs = append(c.logs, entry)
}

// AddMetrics adds metrics from the Prometheus registry
func (c *Collector) AddMetrics(registry prometheus.Gatherer) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.enabled {
		c.logger.Debug().Msg("collector disabled, skipping metrics collection")
		return nil
	}

	// Gather metrics from registry
	mfs, err := registry.Gather()
	if err != nil {
		c.logger.Error().Err(err).Msg("failed to gather metrics from registry")
		return err
	}

	c.logger.Debug().Int("metric_families", len(mfs)).Msg("gathered metrics from registry")

	// Convert to TimeSeries
	now := time.Now()
	timeseries := make([]prompb.TimeSeries, 0, len(mfs)*10) // Estimate capacity

	for _, mf := range mfs {
		for _, metric := range mf.Metric {
			labels := make([]prompb.Label, 0, len(metric.Label)+3)

			// Add standard labels
			labels = append(labels, prompb.Label{
				Name:  "__name__",
				Value: *mf.Name,
			})
			// Don't add source label here as it might conflict with existing labels
			// Instead, we'll add it as a separate label
			labels = append(labels, prompb.Label{
				Name:  "agent_telemetry_source",
				Value: "sm-agent-telemetry-test",
			})
			if c.probeName != "" {
				labels = append(labels, prompb.Label{
					Name:  "probe",
					Value: c.probeName,
				})
			}
			if c.region != "" {
				labels = append(labels, prompb.Label{
					Name:  "region",
					Value: c.region,
				})
			}

			// Add metric labels
			for _, label := range metric.Label {
				labels = append(labels, prompb.Label{
					Name:  *label.Name,
					Value: *label.Value,
				})
			}

			// Get metric value
			var value float64
			switch {
			case metric.Counter != nil:
				value = *metric.Counter.Value
			case metric.Gauge != nil:
				value = *metric.Gauge.Value
			case metric.Histogram != nil:
				// For histograms, we'll create multiple time series
				// This is simplified - in practice you'd want to handle all histogram buckets
				value = float64(*metric.Histogram.SampleCount)
			case metric.Summary != nil:
				value = float64(*metric.Summary.SampleCount)
			default:
				continue
			}

			timeseries = append(timeseries, prompb.TimeSeries{
				Labels: labels,
				Samples: []prompb.Sample{{
					Timestamp: now.UnixMilli(),
					Value:     value,
				}},
			})
		}
	}

	// Add to metrics, maintaining max size
	if len(c.metrics)+len(timeseries) > c.maxMetrics {
		// Remove oldest metrics to make room
		removeCount := len(timeseries)
		if removeCount > len(c.metrics) {
			removeCount = len(c.metrics)
		}
		c.metrics = c.metrics[removeCount:]
	}
	c.metrics = append(c.metrics, timeseries...)

	c.logger.Debug().
		Int("new_timeseries", len(timeseries)).
		Int("total_metrics", len(c.metrics)).
		Msg("added metrics to collector")

	return nil
}

// GetLogs returns collected logs
func (c *Collector) GetLogs() []logproto.Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	logs := make([]logproto.Entry, len(c.logs))
	copy(logs, c.logs)
	return logs
}

// GetMetrics returns collected metrics
func (c *Collector) GetMetrics() []prompb.TimeSeries {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metrics := make([]prompb.TimeSeries, len(c.metrics))
	copy(metrics, c.metrics)
	return metrics
}

// Clear clears collected data
func (c *Collector) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logs = c.logs[:0]
	c.metrics = c.metrics[:0]
}

// formatLogLine formats a log entry as a single line
func (c *Collector) formatLogLine(level, message string, fields map[string]interface{}) string {
	// Simple JSON-like format for now
	// In practice, you might want to use a proper JSON encoder
	line := "level=" + level + " msg=" + message

	for k, v := range fields {
		line += " " + k + "=" + formatValue(v)
	}

	return line
}

// formatValue formats a value for logging
func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return fmt.Sprintf("%d", val)
	case int32:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float32:
		return fmt.Sprintf("%f", val)
	case float64:
		return fmt.Sprintf("%f", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
