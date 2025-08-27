package telemetry

import (
	"context"
	"fmt"
	"sync"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

// RegionPusher periodically sends telemetry data for a specific region.
type RegionPusher struct {
	client sm.TelemetryClient
	logger zerolog.Logger

	instance string
	regionID int32

	telemetry   map[int64]map[sm.CheckClass]*sm.CheckClassTelemetry // Indexed by local tenant ID
	telemetryMu sync.Mutex

	metrics RegionMetrics
}

type RegionMetrics struct {
	pushRequestsActive   prom.Gauge
	pushRequestsDuration prom.Observer
	pushRequestsTotal    prom.Counter
	pushRequestsError    prom.Counter

	addExecutionDuration prom.Observer
}

// start handles region metrics before a push telemetry request.
func (m *RegionMetrics) start() (start time.Time) {
	m.pushRequestsActive.Inc()
	m.pushRequestsTotal.Inc()
	return time.Now()
}

// end handles region metrics after a push telemetry request.
func (m *RegionMetrics) end(err error, start time.Time) {
	m.pushRequestsActive.Dec()
	m.pushRequestsDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		m.pushRequestsError.Inc()
	}
}

// NewRegionPusher builds a new RegionPusher.
// Notice that the effective time span used to dictate the pace for periodic
// push events will be defined based on the given time span plus a random
// jitter [0,59)s.
func NewRegionPusher(
	ctx context.Context, timeSpan time.Duration,
	client sm.TelemetryClient, logger zerolog.Logger, instance string, regionID int32,
	metrics RegionMetrics, options ...RegionPusherOption,
) *RegionPusher {
	tp := &RegionPusher{
		client:    client,
		logger:    logger,
		instance:  instance,
		regionID:  regionID,
		telemetry: make(map[int64]map[sm.CheckClass]*sm.CheckClassTelemetry),
		metrics:   metrics,
	}

	var opts regionPusherOptions

	for _, opt := range options {
		opt(&opts)
	}

	opts.ApplyDefaults(timeSpan)

	opts.wg.Add(1)

	go func() {
		defer opts.wg.Done()
		tp.run(ctx, opts.ticker, opts.wg)
	}()

	return tp
}

type regionPusherOptions struct {
	ticker ticker
	wg     *sync.WaitGroup
}

func (opts *regionPusherOptions) ApplyDefaults(timeSpan time.Duration) {
	if opts.ticker == nil {
		opts.ticker = newStdTicker(timeSpan)
	}

	if opts.wg == nil {
		opts.wg = new(sync.WaitGroup)
	}
}

type RegionPusherOption func(opts *regionPusherOptions)

func WithTicker(t ticker) RegionPusherOption {
	return func(opts *regionPusherOptions) {
		opts.ticker = t
	}
}

func WithWaitGroup(wg *sync.WaitGroup) RegionPusherOption {
	return func(opts *regionPusherOptions) {
		opts.wg = wg
	}
}

func (p *RegionPusher) run(ctx context.Context, ticker ticker, wg *sync.WaitGroup) {
	p.logger.Info().Msg("region pusher starting")
	defer p.logger.Debug().Msg("region pusher stopped")

	// TODO: We could potentially create here a pushCtx, pass that to push, and
	// from push keep retrying sending the data if it fails initially until the
	// pushCtx is canceled, which should happen once the ticker ticks again, to
	// avoid overlapping push requests.
	// By now, only retry folllowing the ticker's pace.

LOOP:
	for {
		select {
		case <-ticker.C():
			p.logger.Info().Msg("pushing telemetry")

			m := p.next()
			wg.Add(1)
			// Avoid blocking
			go func() {
				defer wg.Done()
				p.push(m)
			}()

		case <-ctx.Done():
			p.logger.Debug().Msg("region pusher stopping")

			m := p.next()
			p.push(m)
			ticker.Stop()

			break LOOP
		}
	}
}

// AddExecution adds a new execution to the tenant telemetry.
func (p *RegionPusher) AddExecution(e Execution) {
	start := time.Now()
	p.telemetryMu.Lock()
	defer p.telemetryMu.Unlock()

	tenantTele, ok := p.telemetry[e.LocalTenantID]
	if !ok {
		tenantTele = make(map[sm.CheckClass]*sm.CheckClassTelemetry)
		p.telemetry[e.LocalTenantID] = tenantTele
	}

	clTele, ok := tenantTele[e.CheckClass]
	if !ok {
		clTele = &sm.CheckClassTelemetry{CheckClass: e.CheckClass}
		tenantTele[e.CheckClass] = clTele
	}

	clTele.Executions++
	clTele.Duration += float32(e.Duration.Seconds())
	clTele.SampledExecutions += int32((e.Duration + time.Minute - 1) / time.Minute)

	// measure contention for AddExecution
	p.metrics.addExecutionDuration.Observe(
		time.Since(start).Seconds(),
	)
}

func (p *RegionPusher) next() sm.RegionTelemetry {
	m := sm.RegionTelemetry{
		Instance:  p.instance,
		RegionId:  p.regionID,
		Telemetry: make([]*sm.TenantTelemetry, 0, len(p.telemetry)),
	}

	p.telemetryMu.Lock()
	defer p.telemetryMu.Unlock()

	// Copy current telemetry data
	for tenantID, tTele := range p.telemetry {
		tenantTele := &sm.TenantTelemetry{
			TenantId:  tenantID,
			Telemetry: make([]*sm.CheckClassTelemetry, 0, len(tTele)),
		}
		for _, clTele := range tTele {
			tenantTele.Telemetry = append(tenantTele.Telemetry, &sm.CheckClassTelemetry{
				CheckClass:        clTele.CheckClass,
				Executions:        clTele.Executions,
				Duration:          clTele.Duration,
				SampledExecutions: clTele.SampledExecutions,
			})
		}
		m.Telemetry = append(m.Telemetry, tenantTele)
	}

	return m
}

func (p *RegionPusher) push(m sm.RegionTelemetry) {
	var (
		r   *sm.PushTelemetryResponse
		err error
	)

	start := p.metrics.start()
	defer func() { p.metrics.end(err, start) }()

	// We don't want to cancel a possibly ongoing request even if the agent
	// context is done, therefore use background context
	ctx := context.Background()
	r, err = p.client.PushTelemetry(ctx, &m)
	if err != nil {
		p.logger.Err(err).Msg("error pushing telemetry")
		return
	}
	if r.Status.Code != sm.StatusCode_OK {
		// create an error so it's handled by metrics.end() on defer
		err = fmt.Errorf("unexpected status code")
		p.logger.Err(err).
			Int32("statusCode", int32(r.Status.Code)).
			Str("statusMessage", r.Status.Message).
			Msg("error pushing telemetry")
	}
}
