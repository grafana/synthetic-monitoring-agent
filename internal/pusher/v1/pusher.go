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
	ctx            context.Context
	tenantManager  pusher.TenantProvider
	logger         zerolog.Logger
	clientsMutex   sync.Mutex
	clients        map[int64]*remoteTarget
	pushCounter    *prometheus.CounterVec
	errorCounter   *prometheus.CounterVec
	bytesOut       *prometheus.CounterVec
	failedCounter  *prometheus.CounterVec
	retriesCounter *prometheus.CounterVec
}

var _ pusher.Publisher = &publisherImpl{}

func NewPublisher(ctx context.Context, tm pusher.TenantProvider, logger zerolog.Logger, promRegisterer prometheus.Registerer) pusher.Publisher {
	pushCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "publisher",
			Name:      "push_total",
			Help:      "Total number of push events.",
		},
		[]string{"type", "regionID", "tenantID"})

	promRegisterer.MustRegister(pushCounter)

	errorCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "publisher",
			Name:      "push_errors_total",
			Help:      "Total number of push errors.",
		},
		[]string{"type", "regionID", "tenantID", "status"})

	promRegisterer.MustRegister(errorCounter)

	failedCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "publisher",
			Name:      "push_failed_total",
			Help:      "Total number of push failed.",
		},
		[]string{"type", "regionID", "tenantID"})

	promRegisterer.MustRegister(failedCounter)

	bytesOut := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "publisher",
			Name:      "push_bytes",
			Help:      "Total number of bytes pushed.",
		},
		[]string{"target", "regionID", "tenantID"})

	promRegisterer.MustRegister(bytesOut)

	retriesCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "publisher",
			Name:      "retries_total",
			Help:      "Total number of retries performed.",
		},
		[]string{"target", "regionID", "tenantID"})

	promRegisterer.MustRegister(retriesCounter)

	return &publisherImpl{
		ctx:            ctx,
		tenantManager:  tm,
		clients:        make(map[int64]*remoteTarget),
		logger:         logger,
		pushCounter:    pushCounter,
		errorCounter:   errorCounter,
		bytesOut:       bytesOut,
		failedCounter:  failedCounter,
		retriesCounter: retriesCounter,
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
		localID, regionID = pusher.GetLocalAndRegionIDs(tenantID)
		regionStr         = strconv.FormatInt(int64(regionID), 10)
		tenantStr         = strconv.FormatInt(localID, 10)

		newClient = false
		logger    = p.logger.With().Int("region", regionID).Int64("tenant", localID).Logger()
	)

	for retry := 2; retry > 0; retry-- {
		client, err := p.getClient(ctx, tenantID, newClient)
		if err != nil {
			logger.Error().Err(err).Msg("get client failed")
			p.failedCounter.WithLabelValues("client", regionStr, tenantStr).Inc()
			return
		}

		if len(payload.Streams()) > 0 {
			if n, err := p.pushEvents(ctx, client.Events, payload.Streams()); err != nil {
				httpStatusCode, hasStatusCode := prom.GetHttpStatusCode(err)
				logger.Error().Err(err).Int("status", httpStatusCode).Msg("publish events")
				p.errorCounter.WithLabelValues("logs", regionStr, tenantStr, strconv.Itoa(httpStatusCode)).Inc()
				if hasStatusCode && httpStatusCode == http.StatusUnauthorized {
					// Retry to get a new client, credentials might be stale.
					newClient = true
					continue
				}
			} else {
				p.pushCounter.WithLabelValues("logs", regionStr, tenantStr).Inc()
				p.bytesOut.WithLabelValues("logs", regionStr, tenantStr).Add(float64(n))
			}
		}

		if len(payload.Metrics()) > 0 {
			if n, err := p.pushMetrics(ctx, client.Metrics, payload.Metrics()); err != nil {
				httpStatusCode, hasStatusCode := prom.GetHttpStatusCode(err)
				logger.Error().Err(err).Int("status", httpStatusCode).Msg("publish metrics")
				p.errorCounter.WithLabelValues("metrics", regionStr, tenantStr, strconv.Itoa(httpStatusCode)).Inc()
				if hasStatusCode && httpStatusCode == http.StatusUnauthorized {
					// Retry to get a new client, credentials might be stale.
					newClient = true
					continue
				}
			} else {
				p.pushCounter.WithLabelValues("metrics", regionStr, tenantStr).Inc()
				p.bytesOut.WithLabelValues("metrics", regionStr, tenantStr).Add(float64(n))
			}
		}

		// if we make it here we have sent everything we could send, we are done.
		return
	}

	// if we are here, we retried and failed
	p.failedCounter.WithLabelValues("retry_exhausted", regionStr, tenantStr).Inc()
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

func (p *publisherImpl) getClient(ctx context.Context, tenantId int64, newClient bool) (*remoteTarget, error) {
	var (
		client *remoteTarget
		found  bool
	)

	localID, regionID := pusher.GetLocalAndRegionIDs(tenantId)

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
		Id: tenantId,
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

	localID, regionID := pusher.GetLocalAndRegionIDs(tenant.Id)

	regionStr := strconv.FormatInt(int64(regionID), 10)
	tenantStr := strconv.FormatInt(localID, 10)

	mClient, err := prom.NewClient(tenant.MetricsRemote.Name, mClientCfg, func(c float64) {
		p.retriesCounter.WithLabelValues("metrics", regionStr, tenantStr).Add(c)
	})
	if err != nil {
		return nil, fmt.Errorf("creating metrics client: %w", err)
	}

	eClientCfg, err := pusher.ClientFromRemoteInfo(tenant.EventsRemote)
	if err != nil {
		return nil, fmt.Errorf("creating events client configuration: %w", err)
	}

	eClient, err := prom.NewClient(tenant.EventsRemote.Name, eClientCfg, func(c float64) {
		p.retriesCounter.WithLabelValues("logs", regionStr, tenantStr).Add(c)
	})
	if err != nil {
		return nil, fmt.Errorf("creating events client: %w", err)
	}

	clients := &remoteTarget{
		Metrics: mClient,
		Events:  eClient,
	}

	p.clientsMutex.Lock()
	p.clients[tenant.Id] = clients
	p.clientsMutex.Unlock()
	p.logger.Debug().Int("regionId", regionID).Int64("tenantId", localID).Int64("stackId", tenant.StackId).Msg("updated client")

	return clients, nil
}
