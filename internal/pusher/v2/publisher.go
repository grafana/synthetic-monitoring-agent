package v2

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
)

const Name = "v2"

// NewPublisher creates a new instance of the v2 Publisher.
//
// The provider context is used to control the lifetime of the publisher.
func NewPublisher(ctx context.Context, tenantProvider pusher.TenantProvider, logger zerolog.Logger, pr prometheus.Registerer) pusher.Publisher {
	impl := &publisherImpl{
		ctx:            ctx,
		tenantProvider: tenantProvider,
		options:        defaultPusherOptions,
		handlers:       make(map[model.GlobalID]payloadHandler),
	}
	impl.options.logger = logger
	impl.options.metrics = pusher.NewMetrics(pr)

	return impl
}

type payloadHandler interface {
	publish(payload pusher.Payload)
	// run returns the handler that should run or nil to signal that it
	// should be terminated.
	run(ctx context.Context) payloadHandler
}

type publisherImpl struct {
	ctx            context.Context
	tenantProvider pusher.TenantProvider
	options        pusherOptions
	handlerMutex   sync.Mutex // protects the handlers map
	handlers       map[model.GlobalID]payloadHandler
}

var _ pusher.Publisher = &publisherImpl{}

func (p *publisherImpl) Publish(payload pusher.Payload) {
	tenantID := payload.Tenant()
	handler, found := p.getHandler(tenantID)
	if !found {
		var swapped bool
		newHandler := newTenantPusher(tenantID, p.tenantProvider, p.options.withTenant(tenantID))
		handler, swapped = p.replaceHandler(tenantID, nil, newHandler)
		if swapped {
			go p.runHandler(tenantID, handler)
		}
	}
	handler.publish(payload)
}

func (p *publisherImpl) runHandler(tenantID model.GlobalID, h payloadHandler) {
	tid, rid := model.GetLocalAndRegionIDs(tenantID)
	p.options.logger.Info().Int64("tenant_id", tid).Int("region_id", rid).Msg("started push handler")
	defer p.options.logger.Info().Int64("tenant_id", tid).Int("region_id", rid).Msg("stopped push handler")

	for ok := true; ok && h != nil; {
		next := h.run(p.ctx)
		h, ok = p.replaceHandler(tenantID, h, next)
		if !ok {
			p.options.logger.Error().Int64("tenant_id", tid).Int("region_id", rid).Msg("unable to swap handler, tenant hijacked")
		}
	}
}

// replaceHandler exchanges the old handler with the new handler for the tenant
// identified by tenantID.
//
// By passing a nil new handler, you are trying to delete. A nil old handler
// means you are trying to add.
//
// The handler currently in effect is returned, along with whether the handler
// was changed or not.
func (p *publisherImpl) replaceHandler(tenantID model.GlobalID, old, new payloadHandler) (payloadHandler, bool) {
	p.handlerMutex.Lock()
	defer p.handlerMutex.Unlock()

	// Get the existing handler if any.
	current := p.handlers[tenantID]

	//  old     | current | new     | op
	//  --------+---------+---------+---------------
	//  nil     | nil     | nil     | delete (noop)
	//  nil     | nil     | non-nil | add
	// *nil     | non-nil | nil     | delete
	// *nil     | non-nil | non-nil | replace
	// *non-nil | nil     | nil     | delete (noop)
	// *non-nil | nil     | non-nil | add
	//  non-nil | non-nil | nil     | delete
	//  non-nil | non-nil | non-nil | replace

	// If old is nil, that means we are trying to add a handler. If current
	// is not nil, that means there's an existing handler, and the addition
	// is not necessary. If current is nil, we go ahead and add the new handler.
	//
	// If old is not nil, we are trying to replace or delete a handler.
	//
	// If there's nothing there, current is nil and therefore different from
	// old. That means there's nothing to replace (somebody beat us to it)
	// and nothing to delete (idem).
	//
	// If there's something there, current is not nil, we have to replace
	// the existing one.
	if current != old {
		return current, false
	}

	if new != nil {
		p.handlers[tenantID] = new
	} else {
		delete(p.handlers, tenantID)
	}

	p.options.metrics.InstalledHandlers.Set(float64(len(p.handlers)))

	return new, true
}

func (p *publisherImpl) getHandler(tenantID model.GlobalID) (payloadHandler, bool) {
	p.handlerMutex.Lock()
	defer p.handlerMutex.Unlock()

	handler, found := p.handlers[tenantID]
	return handler, found
}
