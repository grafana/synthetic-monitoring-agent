package pusher

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"

	logproto "github.com/grafana/loki/pkg/push"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type Payload interface {
	Tenant() model.GlobalID
	Metrics() []prompb.TimeSeries
	Streams() []logproto.Stream
}

type Publisher interface {
	Publish(Payload)
}

type TenantProvider interface {
	GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error)
}

type Factory func(ctx context.Context, tm TenantProvider, logger zerolog.Logger, promRegisterer prometheus.Registerer) Publisher
