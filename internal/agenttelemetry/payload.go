package agenttelemetry

import (
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/prometheus/prometheus/prompb"

	logproto "github.com/grafana/loki/pkg/push"
)

// AgentPayload implements pusher.Payload for agent telemetry data
type AgentPayload struct {
	tenantID model.GlobalID
	streams  []logproto.Stream
	metrics  []prompb.TimeSeries
}

// NewAgentPayload creates a new agent telemetry payload
func NewAgentPayload(tenantID model.GlobalID, logs []logproto.Entry, metrics []prompb.TimeSeries, probeName, region string) *AgentPayload {
	// Create log stream with agent-specific labels
	labels := map[string]string{
		"source": "sm-agent-telemetry-test",
		"type":   "agent",
	}

	if probeName != "" {
		labels["probe"] = probeName
	}
	if region != "" {
		labels["region"] = region
	}

	stream := logproto.Stream{
		Labels:  formatLabels(labels),
		Entries: logs,
	}

	return &AgentPayload{
		tenantID: tenantID,
		streams:  []logproto.Stream{stream},
		metrics:  metrics,
	}
}

// Tenant returns the tenant ID
func (p *AgentPayload) Tenant() model.GlobalID {
	return p.tenantID
}

// Streams returns the log streams
func (p *AgentPayload) Streams() []logproto.Stream {
	return p.streams
}

// Metrics returns the metrics
func (p *AgentPayload) Metrics() []prompb.TimeSeries {
	return p.metrics
}

// formatLabels formats labels as a Loki label string
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "{}"
	}

	result := "{"
	first := true
	for k, v := range labels {
		if !first {
			result += ","
		}
		result += k + "=" + `"` + v + `"`
		first = false
	}
	result += "}"
	return result
}
