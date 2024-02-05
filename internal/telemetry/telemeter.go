package telemetry

import (
	"context"
	"sync"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
)

// Telemeter maintains the telemetry data for all the tenants running checks
// in the agent instance, organized by region.
type Telemeter struct {
	ctx    context.Context
	client sm.TelemetryClient
	logger zerolog.Logger

	instance     string
	pushTimeSpan time.Duration

	pushers   map[int32]*RegionPusher // Indexed by region ID
	pushersMu sync.RWMutex
}

// Execution represents the telemetry for a check execution.
type Execution struct {
	LocalTenantID int64
	RegionID      int32
	CheckClass    sm.CheckClass
	Duration      time.Duration
}

// NewTelemeter creates a new Telemeter component.
func NewTelemeter(
	ctx context.Context, instance string, pushTimeSpan time.Duration, client sm.TelemetryClient, logger zerolog.Logger,
) *Telemeter {
	t := &Telemeter{
		ctx:          ctx,
		client:       client,
		logger:       logger,
		instance:     instance,
		pushTimeSpan: pushTimeSpan,
		pushers:      make(map[int32]*RegionPusher),
	}

	return t
}

// AddExecution adds a new execution to the agent's telemetry.
func (t *Telemeter) AddExecution(e Execution) {
	t.pushersMu.RLock()
	if p, ok := t.pushers[e.RegionID]; ok {
		p.AddExecution(e)
		t.pushersMu.RUnlock()
		return
	}
	t.pushersMu.RUnlock()

	// If we do not have a pusher for this region, create it
	l := t.logger.With().
		Str("component", "region-pusher").
		Str("instance", t.instance).
		Int32("regionId", e.RegionID).
		Logger()
	p := NewRegionPusher(
		t.ctx, t.pushTimeSpan,
		t.client, l, t.instance, e.RegionID,
	)
	p.AddExecution(e)

	t.pushersMu.Lock()
	defer t.pushersMu.Unlock()
	t.pushers[e.RegionID] = p
}
