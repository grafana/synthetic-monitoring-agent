package v2

import (
	"context"
	"errors"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/logproto"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// tenantPusher is in charge of pushing changes for a specific tenant.
type tenantPusher struct {
	tenantID       model.GlobalID
	pushCounter    uint64 // FIXME(mem)
	logs, metrics  queue
	tenantProvider pusher.TenantProvider
	options        pusherOptions
}

var _ payloadHandler = &tenantPusher{}

func newTenantPusher(tenantID model.GlobalID, tenantProvider pusher.TenantProvider, options pusherOptions) *tenantPusher {
	mOptions := options.withType(pusher.LabelValueMetrics)
	eOptions := options.withType(pusher.LabelValueLogs)
	tp := &tenantPusher{
		tenantID:       tenantID,
		tenantProvider: tenantProvider,
		options:        options,
		logs:           newQueue(&eOptions),
		metrics:        newQueue(&mOptions),
	}
	return tp
}

func (p *tenantPusher) run(ctx context.Context) payloadHandler {
	p.options.logger.Info().Msg("starting pusher")
	err := p.runPushers(ctx)
	p.options.logger.Info().Err(err).Msg("pusher terminated")

	if err == nil || err == context.Canceled || err == errTenantIdle {
		p.options.logger.Info().Err(err).Msg("clearing tenant slot")
		return nil
	}

	var pErr pushError
	if !errors.As(err, &pErr) {
		p.options.logger.Warn().Err(err).Msg("unexpected error, clearing slot")
		return nil
	}

	switch pErr.kind {
	case errKindWait:
		// Some failure forces us to stop pushing for a while, but keep accumulating metrics meanwhile.
		p.options.logger.Info().Dur("delay", p.options.waitPeriod).Msg("delaying metrics publishing")
		return &delayPusher{
			next:  p,
			delay: p.options.waitPeriod,
		}

	case errKindTenant:
		p.options.logger.Debug().Msg("refreshing tenant")
		// The tenant could be stale. Let's just restart the pusher with a minimal delay.
		return &delayPusher{
			next:  p,
			delay: p.options.tenantDelay,
		}

	case errKindFatal:
		// Possibly a situation where we can't push metrics and won't be able to push them
		// in the future. Discard metrics for this tenant for an extended period of time.
		p.options.logger.Warn().Dur("duration", p.options.discardPeriod).Msg("discarding metrics")
		return &discardPusher{
			duration: p.options.discardPeriod,
			options:  p.options,
		}

	default:
		p.options.logger.Warn().Err(err).Msg("unexpected error, clearing slot")
		return nil
	}
}

func (p *tenantPusher) runPushers(ctx context.Context) error {
	// TODO(adrian): If tenant had the plan in here, we could have different retention for paid tenants.
	tenant, err := p.tenantProvider.GetTenant(ctx, &sm.TenantInfo{
		Id: int64(p.tenantID),
	})
	if err != nil {
		p.options.metrics.FailedCounter.With(prometheus.Labels{"type": pusher.LabelValueMetrics, "reason": pusher.LabelValueTenant}).Inc()
		p.options.metrics.FailedCounter.With(prometheus.Labels{"type": pusher.LabelValueLogs, "reason": pusher.LabelValueTenant}).Inc()
		return pushError{
			kind:  errKindFatal,
			inner: err,
		}
	}

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return p.metrics.push(gCtx, tenant.MetricsRemote)
	})

	g.Go(func() error {
		return p.logs.push(gCtx, tenant.EventsRemote)
	})

	// Optionally perform some validations.

	if p.options.maxIdleTime > 0 {
		g.Go(idleChecker(gCtx, p.options.maxIdleTime, &p.pushCounter))
	}

	if p.options.maxLifetime > 0 {
		// generate a random number [-maxLifetimeJitter/2, maxLifetimeJitter/2)
		jitter := (rand.Float64() - 0.5) * p.options.maxLifetimeJitter
		// adjust maxLifetime by that amount
		maxLifetime := p.options.maxLifetime + time.Duration(jitter*float64(p.options.maxLifetime))
		g.Go(maxLifetimeChecker(gCtx, maxLifetime))
	}

	return g.Wait()
}

var (
	errMaxLifeTimeExceeded = pushError{
		kind:  errKindTenant,
		inner: errors.New("max life time exceeded"),
	}
	errTenantIdle = errors.New("tenant is idle")
)

func idleChecker(ctx context.Context, interval time.Duration, ptr *uint64) func() error {
	return func() error {
		t := time.NewTicker(interval)
		defer t.Stop()
		lastValue := atomic.LoadUint64(ptr)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				// check that source has changed
				curValue := atomic.LoadUint64(ptr)
				if curValue == lastValue {
					return errTenantIdle
				}
				lastValue = curValue
			}
		}
	}
}

func maxLifetimeChecker(ctx context.Context, maxLifetime time.Duration) func() error {
	return func() error {
		t := time.NewTimer(maxLifetime)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
			return errMaxLifeTimeExceeded
		}
	}
}

func (p *tenantPusher) publish(payload pusher.Payload) {
	atomic.AddUint64(&p.pushCounter, 1)

	if len(payload.Metrics()) > 0 {
		p.metrics.insert(toRequest(&prompb.WriteRequest{Timeseries: payload.Metrics()}, p.options.pool))
	}

	if len(payload.Streams()) > 0 {
		p.logs.insert(toRequest(&logproto.PushRequest{Streams: payload.Streams()}, p.options.pool))
	}
}

func toRequest(m proto.Marshaler, p bufferPool) *[]byte {
	data, err := m.Marshal()
	if err != nil {
		panic(err)
	}
	bufPtr := p.get()
	*bufPtr = (*bufPtr)[0:cap(*bufPtr)]
	encoded := snappy.Encode(*bufPtr, data)
	return &encoded
}

type delayPusher struct {
	next  payloadHandler
	delay time.Duration
}

var _ payloadHandler = delayPusher{}

func (p delayPusher) run(ctx context.Context) payloadHandler {
	err := sleepCtx(ctx, p.delay)
	if err != nil {
		// Context cancelled
		return nil
	}

	return p.next
}

func (p delayPusher) publish(payloads pusher.Payload) {
	p.next.publish(payloads)
}

type discardPusher struct {
	duration time.Duration
	options  pusherOptions
}

var _ payloadHandler = discardPusher{}

func (p discardPusher) run(ctx context.Context) payloadHandler {
	// error can be ignored (only possible is context.Canceled)
	// as it's returning a nil handler anyway.
	_ = sleepCtx(ctx, p.duration)
	return nil
}

func (p discardPusher) publish(payloads pusher.Payload) {
	if len(payloads.Metrics()) > 0 {
		p.options.metrics.DroppedCounter.WithLabelValues(pusher.LabelValueMetrics).Inc()
	}
	if len(payloads.Streams()) > 0 {
		p.options.metrics.DroppedCounter.WithLabelValues(pusher.LabelValueLogs).Inc()
	}
}
