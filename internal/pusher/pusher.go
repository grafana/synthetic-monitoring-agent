package pusher

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/grafana/loki/pkg/logproto"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/loki"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/prom"
	"github.com/grafana/worldping-blackbox-sidecar/pkg/pb/worldping"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

const (
	defaultBufferCapacity = 10 * 1024
	userAgent             = "worldping-blackbox-sidecar/0.0.1"
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

type Payload interface {
	Tenant() int64
	Metrics() []prompb.TimeSeries
	Streams() []logproto.Stream
}

type Publisher struct {
	tenantsClient worldping.TenantsClient
	logger        zerolog.Logger
	publishCh     <-chan Payload
	clientsMutex  sync.Mutex
	clients       map[int64]*remoteTarget
	pushCounter   *prometheus.CounterVec
	errorCounter  *prometheus.CounterVec
	bytesOut      *prometheus.CounterVec
}

func NewPublisher(conn *grpc.ClientConn, publishCh <-chan Payload, logger zerolog.Logger, promRegisterer prometheus.Registerer) *Publisher {
	pushCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "worldping_bbe_sidecar",
			Subsystem: "publisher",
			Name:      "push_total",
			Help:      "Total number of push events.",
		},
		[]string{"type", "tenantID"})

	promRegisterer.MustRegister(pushCounter)

	errorCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "worldping_bbe_sidecar",
			Subsystem: "publisher",
			Name:      "push_errors_total",
			Help:      "Total number of push errors.",
		},
		[]string{"type", "tenantID"})

	promRegisterer.MustRegister(errorCounter)

	bytesOut := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "worldping_bbe_sidecar",
			Subsystem: "publisher",
			Name:      "push_bytes",
			Help:      "Total number of bytes pushed.",
		},
		[]string{"target", "tenantID"})

	promRegisterer.MustRegister(bytesOut)

	return &Publisher{
		tenantsClient: worldping.NewTenantsClient(conn),
		publishCh:     publishCh,
		clients:       make(map[int64]*remoteTarget),
		logger:        logger,
		pushCounter:   pushCounter,
		errorCounter:  errorCounter,
		bytesOut:      bytesOut,
	}
}

func (p *Publisher) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case payload := <-p.publishCh:
			go p.publish(ctx, payload)
		}
	}
}

func (p *Publisher) publish(ctx context.Context, payload Payload) {
	tenantID := payload.Tenant()
	tenantStr := strconv.FormatInt(tenantID, 10)

	client, err := p.getClient(ctx, tenantID)
	if err != nil {
		p.logger.Error().Err(err).Int64("tenant", tenantID).Msg("get client")
		p.errorCounter.WithLabelValues("client", tenantStr).Inc()
		return
	}

	if len(payload.Streams()) > 0 {
		if n, err := p.pushEvents(ctx, client.Events, payload.Streams()); err != nil {
			p.logger.Error().Err(err).Int64("tenant", tenantID).Msg("publish events")
			p.errorCounter.WithLabelValues("logs", tenantStr).Inc()
		} else {
			p.pushCounter.WithLabelValues("logs", tenantStr).Inc()
			p.bytesOut.WithLabelValues("logs", tenantStr).Add(float64(n))
		}
	}

	if len(payload.Metrics()) > 0 {
		if n, err := p.pushMetrics(ctx, client.Metrics, payload.Metrics()); err != nil {
			p.logger.Error().Err(err).Int64("tenant", tenantID).Msg("publish metrics")
			p.errorCounter.WithLabelValues("metrics", tenantStr).Inc()
		} else {
			p.pushCounter.WithLabelValues("metrics", tenantStr).Inc()
			p.bytesOut.WithLabelValues("metrics", tenantStr).Add(float64(n))
		}
	}
}

func (p *Publisher) pushEvents(ctx context.Context, client *prom.Client, streams []logproto.Stream) (int, error) {
	buf := bufPool.Get().(*[]byte)
	defer bufPool.Put(buf)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := loki.SendStreamsWithBackoff(ctx, client, streams, buf); err != nil {
		return 0, fmt.Errorf("sending events: %w", err)
	}

	return len(*buf), nil
}

func (p *Publisher) pushMetrics(ctx context.Context, client *prom.Client, metrics []prompb.TimeSeries) (int, error) {
	buf := bufPool.Get().(*[]byte)
	defer bufPool.Put(buf)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := prom.SendSamplesWithBackoff(ctx, client, metrics, buf); err != nil {
		return 0, fmt.Errorf("sending timeseries: %w", err)
	}

	return len(*buf), nil
}

func (p *Publisher) getClient(ctx context.Context, tenantId int64) (*remoteTarget, error) {
	p.clientsMutex.Lock()

	client, found := p.clients[tenantId]
	p.clientsMutex.Unlock()
	if found {
		return client, nil
	}

	req := worldping.TenantInfo{
		Id: tenantId,
	}
	tenant, err := p.tenantsClient.GetTenant(ctx, &req)
	if err != nil {
		return nil, err
	}
	mClientCfg, err := clientFromRemoteInfo(tenantId, tenant.MetricsRemote)
	if err != nil {
		return nil, fmt.Errorf("creating metrics client configuration: %w", err)
	}
	mClient, err := prom.NewClient(tenant.MetricsRemote.Name, mClientCfg)
	if err != nil {
		return nil, fmt.Errorf("creating metrics client: %w", err)
	}
	eClientCfg, err := clientFromRemoteInfo(tenantId, tenant.EventsRemote)
	if err != nil {
		return nil, fmt.Errorf("creating events client configuration: %w", err)
	}
	eClient, err := prom.NewClient(tenant.EventsRemote.Name, eClientCfg)
	if err != nil {
		return nil, fmt.Errorf("creating events client: %w", err)
	}
	clients := &remoteTarget{
		Metrics: mClient,
		Events:  eClient,
	}
	p.clientsMutex.Lock()
	p.clients[tenantId] = clients
	p.clientsMutex.Unlock()

	return clients, nil
}

func clientFromRemoteInfo(tenantId int64, remote *worldping.RemoteInfo) (*prom.ClientConfig, error) {
	// TODO(mem): this is hacky.
	//
	// it's trying to deal with the fact that the URL shown to users
	// is not the push URL but the base for the API endpoints
	u, err := url.Parse(remote.Url + "/push")
	if err != nil {
		// XXX(mem): do we really return an error here?
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	clientCfg := prom.ClientConfig{
		URL:       u,
		Timeout:   5 * time.Second,
		UserAgent: userAgent,
	}

	if remote.Username != "" {
		clientCfg.HTTPClientConfig.BasicAuth = &prom.BasicAuth{
			Username: remote.Username,
			Password: remote.Password,
		}
	}

	if clientCfg.Headers == nil {
		clientCfg.Headers = make(map[string]string)
	}

	clientCfg.Headers["X-Prometheus-Remote-Write-Version"] = "0.1.0"
	clientCfg.Headers["X-Scope-OrgID"] = strconv.FormatInt(tenantId, 10)

	return &clientCfg, nil
}
