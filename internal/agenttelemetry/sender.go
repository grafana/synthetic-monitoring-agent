package agenttelemetry

import (
	"context"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/rs/zerolog"
)

// Sender handles sending agent telemetry data to customer instances
type Sender struct {
	collector     *Collector
	publisher     pusher.Publisher
	tenantManager pusher.TenantProvider
	logger        zerolog.Logger
	interval      time.Duration
	enabled       bool
}

// NewSender creates a new agent telemetry sender
func NewSender(collector *Collector, publisher pusher.Publisher, tenantManager pusher.TenantProvider, logger zerolog.Logger, interval time.Duration) *Sender {
	return &Sender{
		collector:     collector,
		publisher:     publisher,
		tenantManager: tenantManager,
		logger:        logger,
		interval:      interval,
	}
}

// Enable enables the sender
func (s *Sender) Enable() {
	s.enabled = true
	s.logger.Info().Msg("agent telemetry sender enabled")
}

// Disable disables the sender
func (s *Sender) Disable() {
	s.enabled = false
	s.logger.Info().Msg("agent telemetry sender disabled")
}

// Start starts the sender in a goroutine
func (s *Sender) Start(ctx context.Context) {
	if !s.enabled {
		return
	}

	go s.run(ctx)
}

// run is the main loop for sending agent telemetry
func (s *Sender) run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendTelemetry(ctx)
		}
	}
}

// sendTelemetry sends collected agent telemetry data
func (s *Sender) sendTelemetry(ctx context.Context) {
	if !s.enabled {
		return
	}

	// Get collected data
	logs := s.collector.GetLogs()
	metrics := s.collector.GetMetrics()

	s.logger.Debug().
		Int("logs_count", len(logs)).
		Int("metrics_count", len(metrics)).
		Msg("checking for agent telemetry data")

	if len(logs) == 0 && len(metrics) == 0 {
		s.logger.Debug().Msg("no agent telemetry data to send")
		return // Nothing to send
	}

	// Get a valid tenant ID from the tenant manager
	// For now, we'll use the first available tenant
	// In practice, you might want to send to specific tenants or have tenant-specific configuration
	tenantID, err := s.getValidTenantID()
	if err != nil {
		s.logger.Debug().Err(err).Msg("no valid tenant available for agent telemetry")
		return
	}

	// Create payload and send
	payload := NewAgentPayload(tenantID, logs, metrics, s.collector.probeName, s.collector.region)

	s.logger.Info().
		Int("logs", len(logs)).
		Int("metrics", len(metrics)).
		Str("tenant_id", string(tenantID)).
		Msg("publishing agent telemetry payload")

	s.publisher.Publish(payload)

	// Clear sent data
	s.collector.Clear()

	s.logger.Info().
		Int("logs", len(logs)).
		Int("metrics", len(metrics)).
		Msg("sent agent telemetry")
}

// getValidTenantID gets a valid tenant ID from the tenant manager
func (s *Sender) getValidTenantID() (model.GlobalID, error) {
	// TODO: FIX
	return model.GlobalID(3419), nil
}
