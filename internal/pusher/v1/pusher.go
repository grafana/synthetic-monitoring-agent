package v1

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/logproto"
	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/loki"
	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/prom"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

const (
	defaultBufferCapacity = 10 * 1024
	Name                  = "v1"
)

var bufPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 0, defaultBufferCapacity)
		return &buf
	},
}

type remoteTarget struct {
	Events  *prom.Client
	Metrics *prom.Client
}

type publisherImpl struct {
	ctx           context.Context
	tenantManager pusher.TenantProvider
	logger        zerolog.Logger
	clientsMutex  sync.Mutex
	clients       map[model.GlobalID]*remoteTarget
	metrics       pusher.Metrics
}

var _ pusher.Publisher = &publisherImpl{}

func NewPublisher(ctx context.Context, tm pusher.TenantProvider, logger zerolog.Logger, promRegisterer prometheus.Registerer) pusher.Publisher {
	return &publisherImpl{
		ctx:           ctx,
		tenantManager: tm,
		clients:       make(map[model.GlobalID]*remoteTarget),
		logger:        logger,
		metrics:       pusher.NewMetrics(promRegisterer),
	}
}

func (p *publisherImpl) Publish(payload pusher.Payload) {
	go p.publish(p.ctx, payload)
}

func (p *publisherImpl) publish(ctx context.Context, payload pusher.Payload) {
	var (
		tenantID = payload.Tenant()

		// The above tenant ID is potentially a global ID. This is valid
		// for using internally but in logs and metrics we want to publish
		// the region and local tenant ID.
		localID, regionID = model.GetLocalAndRegionIDs(tenantID)
		regionStr         = strconv.FormatInt(int64(regionID), 10)
		tenantStr         = strconv.FormatInt(localID, 10)

		newClient = false
		logger    = p.logger.With().Int("region", regionID).Int64("tenant", localID).Logger()
	)

	streams := payload.Streams()
	metrics := payload.Metrics()

	for retry := 2; retry > 0; retry-- {
		client, err := p.getClient(ctx, tenantID, newClient)
		if err != nil {
			logger.Error().Err(err).Msg("get client failed")
			if len(streams) > 0 {
				p.metrics.FailedCounter.WithLabelValues(regionStr, tenantStr, pusher.LabelValueLogs, pusher.LabelValueClient).Inc()
			}
			if len(metrics) > 0 {
				p.metrics.FailedCounter.WithLabelValues(regionStr, tenantStr, pusher.LabelValueMetrics, pusher.LabelValueClient).Inc()
			}
			return
		}

		if len(streams) > 0 {
			if n, err := p.pushEvents(ctx, client.Events, streams); err != nil {
				httpStatusCode, hasStatusCode := prom.GetHttpStatusCode(err)
				logger.Error().Err(err).Int("status", httpStatusCode).Msg("publish events")
				p.metrics.ErrorCounter.WithLabelValues(regionStr, tenantStr, pusher.LabelValueLogs, strconv.Itoa(httpStatusCode)).Inc()
				if hasStatusCode && httpStatusCode == http.StatusUnauthorized {
					// Retry to get a new client, credentials might be stale.
					newClient = true
					continue
				}
			} else {
				p.metrics.PushCounter.WithLabelValues(regionStr, tenantStr, pusher.LabelValueLogs).Inc()
				p.metrics.BytesOut.WithLabelValues(regionStr, tenantStr, pusher.LabelValueLogs).Add(float64(n))
				streams = nil
			}
		}

		if len(metrics) > 0 {
			if n, err := p.pushMetrics(ctx, client.Metrics, metrics); err != nil {
				httpStatusCode, hasStatusCode := prom.GetHttpStatusCode(err)
				logger.Error().Err(err).Int("status", httpStatusCode).Msg("publish metrics")
				p.metrics.ErrorCounter.WithLabelValues(regionStr, tenantStr, pusher.LabelValueMetrics, strconv.Itoa(httpStatusCode)).Inc()
				if hasStatusCode && httpStatusCode == http.StatusUnauthorized {
					// Retry to get a new client, credentials might be stale.
					newClient = true
					continue
				}
			} else {
				p.metrics.PushCounter.WithLabelValues(regionStr, tenantStr, pusher.LabelValueMetrics).Inc()
				p.metrics.BytesOut.WithLabelValues(regionStr, tenantStr, pusher.LabelValueMetrics).Add(float64(n))
				metrics = nil
			}
		}

		if len(streams) == 0 && len(metrics) == 0 {
			// if we make it here we have sent everything we could send, we are done.
			return
		}
	}

	// if we are here, we retried and failed
	if len(streams) > 0 {
		p.metrics.FailedCounter.WithLabelValues(regionStr, tenantStr, pusher.LabelValueLogs, pusher.LabelValueRetryExhausted).Inc()
	}
	if len(metrics) > 0 {
		p.metrics.FailedCounter.WithLabelValues(regionStr, tenantStr, pusher.LabelValueMetrics, pusher.LabelValueRetryExhausted).Inc()
	}
	logger.Warn().Msg("failed to push payload")
}

func (p *publisherImpl) pushEvents(ctx context.Context, client *prom.Client, streams []logproto.Stream) (int, error) {
	buf := bufPool.Get().(*[]byte)
	defer bufPool.Put(buf)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := loki.SendStreamsWithBackoff(ctx, client, streams, buf); err != nil {
		return 0, fmt.Errorf("sending events: %w", err)
	}

	return len(*buf), nil
}

func (p *publisherImpl) pushMetrics(ctx context.Context, client *prom.Client, metrics []prompb.TimeSeries) (int, error) {
	buf := bufPool.Get().(*[]byte)
	defer bufPool.Put(buf)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := prom.SendSamplesWithBackoff(ctx, client, metrics, buf); err != nil {
		return 0, fmt.Errorf("sending timeseries: %w", err)
	}

	return len(*buf), nil
}

func (p *publisherImpl) getClient(ctx context.Context, tenantId model.GlobalID, newClient bool) (*remoteTarget, error) {
	var (
		client *remoteTarget
		found  bool
	)

	localID, regionID := model.GetLocalAndRegionIDs(tenantId)

	p.clientsMutex.Lock()
	if newClient {
		p.logger.Info().Int("regionId", regionID).Int64("tenantId", localID).Msg("removing tenant from cache")
		delete(p.clients, tenantId)
	} else {
		client, found = p.clients[tenantId]
	}
	p.clientsMutex.Unlock()

	if found {
		return client, nil
	}

	p.logger.Info().Int("regionId", regionID).Int64("tenantId", localID).Msg("fetching tenant credentials")

	req := sm.TenantInfo{
		Id: int64(tenantId),
	}
	tenant, err := p.tenantManager.GetTenant(ctx, &req)
	if err != nil {
		return nil, err
	}

	return p.updateClient(tenant)
}

func (p *publisherImpl) updateClient(tenant *sm.Tenant) (*remoteTarget, error) {
	mClientCfg, err := pusher.ClientFromRemoteInfo(tenant.MetricsRemote)
	if err != nil {
		return nil, fmt.Errorf("creating metrics client configuration: %w", err)
	}

	localID, regionID := model.GetLocalAndRegionIDs(model.GlobalID(tenant.Id))

	regionStr := strconv.FormatInt(int64(regionID), 10)
	tenantStr := strconv.FormatInt(localID, 10)

	mClient, err := prom.NewClient(tenant.MetricsRemote.Name, mClientCfg, func(c float64) {
		p.metrics.RetriesCounter.WithLabelValues(regionStr, tenantStr, pusher.LabelValueMetrics).Add(c)
	})
	if err != nil {
		return nil, fmt.Errorf("creating metrics client: %w", err)
	}

	eClientCfg, err := pusher.ClientFromRemoteInfo(tenant.EventsRemote)
	if err != nil {
		return nil, fmt.Errorf("creating events client configuration: %w", err)
	}

	eClient, err := prom.NewClient(tenant.EventsRemote.Name, eClientCfg, func(c float64) {
		p.metrics.RetriesCounter.WithLabelValues(regionStr, tenantStr, pusher.LabelValueLogs).Add(c)
	})
	if err != nil {
		return nil, fmt.Errorf("creating events client: %w", err)
	}

	clients := &remoteTarget{
		Metrics: mClient,
		Events:  eClient,
	}

	p.clientsMutex.Lock()
	p.clients[model.GlobalID(tenant.Id)] = clients
	p.clientsMutex.Unlock()
	p.logger.Debug().Int("regionId", regionID).Int64("tenantId", localID).Int64("stackId", tenant.StackId).Msg("updated client")

	return clients, nil
}
