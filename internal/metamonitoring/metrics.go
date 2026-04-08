package metamonitoring

import (
	"context"
	"time"

	logproto "github.com/grafana/loki/pkg/push"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
)

const defaultInterval = time.Minute

type HandlerOpts struct {
	Logger    zerolog.Logger
	Registry  *prometheus.Registry
	Publisher pusher.Publisher
	TenantID  model.GlobalID
	Interval  time.Duration
	LogBuffer LogBuffer
}

type metricsHandler struct {
	logger    zerolog.Logger
	registry  *prometheus.Registry
	publisher pusher.Publisher
	tenantID  model.GlobalID
	interval  time.Duration
	logBuffer LogBuffer
}

func NewHandler(opts HandlerOpts) (*metricsHandler, error) {
	interval := opts.Interval
	if interval == 0 {
		interval = defaultInterval
	}

	return &metricsHandler{
		logger:    opts.Logger,
		registry:  opts.Registry,
		publisher: opts.Publisher,
		tenantID:  opts.TenantID,
		interval:  interval,
		logBuffer: opts.LogBuffer,
	}, nil
}

func (m metricsHandler) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case t := <-ticker.C:
			if err := m.reportUsage(); err != nil {
				m.logger.Error().Err(err).Time("t", t).Msg("failed to report metrics")
			} else {
				m.logger.Info().Time("t", t).Msg("reported metrics")
			}
		}
	}
}

func (m metricsHandler) reportUsage() error {
	mfs, err := m.registry.Gather()
	if err != nil {
		return err
	}

	now := time.Now()
	ts := mfsToTimeseries(now, mfs)

	var streams []logproto.Stream
	if m.logBuffer != nil {
		streams = m.logBuffer.Drain()
	}

	if len(ts) == 0 && len(streams) == 0 {
		return nil
	}

	m.publisher.Publish(&payload{
		tenantID: m.tenantID,
		metrics:  ts,
		streams:  streams,
	})

	return nil
}

func mfsToTimeseries(t time.Time, mfs []*dto.MetricFamily) []prompb.TimeSeries {
	stamp := t.UnixNano() / 1e6
	var ts []prompb.TimeSeries

	for _, mf := range mfs {
		name := mf.GetName()
		mType := mf.GetType()

		for _, metric := range mf.GetMetric() {
			ml := metric.GetLabel()
			labels := make([]prompb.Label, 0, 1+len(ml))
			labels = append(labels, prompb.Label{Name: "__name__", Value: name})
			for _, l := range ml {
				labels = append(labels, prompb.Label{Name: l.GetName(), Value: l.GetValue()})
			}

			var value *float64
			switch mType {
			case dto.MetricType_COUNTER:
				if v := metric.GetCounter(); v != nil {
					value = v.Value
				}
			case dto.MetricType_GAUGE:
				if v := metric.GetGauge(); v != nil {
					value = v.Value
				}
			case dto.MetricType_UNTYPED:
				if v := metric.GetUntyped(); v != nil {
					value = v.Value
				}
			}

			if value != nil {
				ts = append(ts, prompb.TimeSeries{
					Labels:  labels,
					Samples: []prompb.Sample{{Timestamp: stamp, Value: *value}},
				})
			}
		}
	}

	return ts
}

type payload struct {
	tenantID model.GlobalID
	metrics  []prompb.TimeSeries
	streams  []logproto.Stream
}

func (p *payload) Tenant() model.GlobalID       { return p.tenantID }
func (p *payload) Metrics() []prompb.TimeSeries { return p.metrics }
func (p *payload) Streams() []logproto.Stream   { return p.streams }
