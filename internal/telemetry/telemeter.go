package telemetry

import (
	"context"
	"strconv"
	"sync"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"

	prom "github.com/prometheus/client_golang/prometheus"
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

	metrics metrics
}

type metrics struct {
	pushRequestsActive   *prom.GaugeVec
	pushRequestsDuration *prom.HistogramVec
	pushRequestsTotal    *prom.CounterVec
	pushRequestsError    *prom.CounterVec

	addExecutionDuration *prom.HistogramVec
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
	ctx context.Context, instance string, pushTimeSpan time.Duration, client sm.TelemetryClient,
	logger zerolog.Logger, registerer prom.Registerer,
) *Telemeter {
	t := &Telemeter{
		ctx:          ctx,
		client:       client,
		logger:       logger,
		instance:     instance,
		pushTimeSpan: pushTimeSpan,
		pushers:      make(map[int32]*RegionPusher),
	}

	t.registerMetrics(registerer)

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

	// There is a minimal time window here on which a concurrent request could
	// acquire the lock for the same region, therefore, acquiere the W lock as
	// soon as the R lock has been released to avoid overlapping work, and
	// verify that once acquired there is still no pusher for the region.
	// This section will only be executed "N regions" times.
	t.pushersMu.Lock()
	defer t.pushersMu.Unlock()
	if p, ok := t.pushers[e.RegionID]; ok {
		p.AddExecution(e)
		return
	}

	// If we do not have a pusher for this region, create it
	l := t.logger.With().
		Str("component", "region-pusher").
		Str("agentInstance", t.instance).
		Int32("regionId", e.RegionID).
		Logger()
	labels := prom.Labels{
		"region_id": strconv.FormatInt(int64(e.RegionID), 10),
	}
	m := RegionMetrics{
		t.metrics.pushRequestsActive.With(labels),
		t.metrics.pushRequestsDuration.With(labels),
		t.metrics.pushRequestsTotal.With(labels),
		t.metrics.pushRequestsError.With(labels),
		t.metrics.addExecutionDuration.With(labels),
	}
	p := NewRegionPusher(
		t.ctx, t.pushTimeSpan, t.client,
		l, t.instance, e.RegionID, m,
	)
	p.AddExecution(e)

	t.pushers[e.RegionID] = p
}

func (t *Telemeter) registerMetrics(registerer prom.Registerer) {
	t.metrics.pushRequestsActive = prom.NewGaugeVec(prom.GaugeOpts{
		Namespace:   "sm_agent",
		Subsystem:   "telemetry",
		Name:        "push_requests_active",
		Help:        "Active push telemetry requests",
		ConstLabels: prom.Labels{"agent_instance": t.instance},
	}, []string{"region_id"})
	t.metrics.pushRequestsDuration = prom.NewHistogramVec(prom.HistogramOpts{
		Namespace:   "sm_agent",
		Subsystem:   "telemetry",
		Name:        "push_requests_duration_seconds",
		Help:        "Duration of push telemetry requests",
		Buckets:     prom.ExponentialBucketsRange(0.01, 2.0, 10),
		ConstLabels: prom.Labels{"agent_instance": t.instance},
	}, []string{"region_id"})
	t.metrics.pushRequestsTotal = prom.NewCounterVec(prom.CounterOpts{
		Namespace:   "sm_agent",
		Subsystem:   "telemetry",
		Name:        "push_requests_total",
		Help:        "Total count of push telemetry requests",
		ConstLabels: prom.Labels{"agent_instance": t.instance},
	}, []string{"region_id"})
	t.metrics.pushRequestsError = prom.NewCounterVec(prom.CounterOpts{
		Namespace:   "sm_agent",
		Subsystem:   "telemetry",
		Name:        "push_requests_errors_total",
		Help:        "Total count of errored push telemetry requests",
		ConstLabels: prom.Labels{"agent_instance": t.instance},
	}, []string{"region_id"})

	t.metrics.addExecutionDuration = prom.NewHistogramVec(prom.HistogramOpts{
		Namespace:                       "sm_agent",
		Subsystem:                       "telemetry",
		Name:                            "add_execution_duration_seconds",
		Help:                            "Duration of add telemetry executions",
		Buckets:                         []float64{.00001, .00002, .00005, .0001, .0002, .0005, .001, .002, .005, .01, .02, .05, .1, .2},
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  100,
		NativeHistogramMinResetDuration: time.Hour,
		ConstLabels:                     prom.Labels{"agent_instance": t.instance},
	}, []string{"region_id"})

	registerer.MustRegister(t.metrics.pushRequestsActive)
	registerer.MustRegister(t.metrics.pushRequestsDuration)
	registerer.MustRegister(t.metrics.pushRequestsTotal)
	registerer.MustRegister(t.metrics.pushRequestsError)
	registerer.MustRegister(t.metrics.addExecutionDuration)
}
