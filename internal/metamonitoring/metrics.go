package metamonitoring

import (
	"context"
	"errors"
	"fmt"
	"time"

	logproto "github.com/grafana/loki/pkg/push"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
)

const defaultInterval = time.Minute

var errTenantTimeout = errors.New("timed out waiting for probes tenant id")

type HandlerOpts struct {
	Logger    zerolog.Logger
	Registry  prometheus.Gatherer
	Publisher pusher.Publisher
	TenantID  model.GlobalID
	Interval  time.Duration
	ProbeCh   chan *synthetic_monitoring.Probe
	LogBuffer LogBuffer
}

type metricsHandler struct {
	logger    zerolog.Logger
	registry  prometheus.Gatherer
	publisher pusher.Publisher
	tenantID  model.GlobalID
	probeName string
	interval  time.Duration
	probeCh   chan *synthetic_monitoring.Probe
	logBuffer LogBuffer
}
type Handler interface {
	Run(ctx context.Context) error
}

func NewHandler(opts HandlerOpts) Handler {
	interval := opts.Interval
	if interval == 0 {
		interval = defaultInterval
	}

	return &metricsHandler{
		logger:    opts.Logger,
		registry:  opts.Registry,
		publisher: opts.Publisher,
		interval:  interval,
		probeCh:   opts.ProbeCh,
		tenantID:  opts.TenantID,
		logBuffer: opts.LogBuffer,
	}
}

func (m *metricsHandler) Run(ctx context.Context) error {
	if m.tenantID == 0 {
		err := m.waitForTenantID(ctx)
		if err != nil {
			return err
		}
	}
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	m.logger = m.logger.With().Int64("tenantID", int64(m.tenantID)).Logger()
	m.logger.Info().Msg("starting to report metrics")
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := m.reportUsage(); err != nil {
				m.logger.Error().Err(err).Msg("failed to report metrics")
			}
		}
	}
}

func (m *metricsHandler) waitForTenantID(ctx context.Context) error {
	select {
	case probe := <-m.probeCh:
		m.tenantID = model.GlobalID(probe.TenantId)
		m.probeName = probe.Name
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w %w", errTenantTimeout, ctx.Err())
	}
}

func (m *metricsHandler) reportUsage() error {
	mfs, err := m.registry.Gather()
	if err != nil {
		return err
	}

	now := time.Now()
	ts := mfsToTimeseries(now, mfs, m.probeName)

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

func mfsToTimeseries(t time.Time, mfs []*dto.MetricFamily, probeName string) []prompb.TimeSeries {
	stamp := t.UnixNano() / 1e6
	var ts []prompb.TimeSeries

	for _, mf := range mfs {
		name := mf.GetName()
		mType := mf.GetType()

		for _, metric := range mf.GetMetric() {
			ml := metric.GetLabel()
			labels := make([]prompb.Label, 0, 1+len(ml))
			labels = append(labels, prompb.Label{Name: "__name__", Value: name})
			labels = append(labels, prompb.Label{Name: "probe", Value: probeName})
			for _, l := range ml {
				labels = append(labels, prompb.Label{Name: l.GetName(), Value: l.GetValue()})
			}

			// Histograms and summaries are intentionally skipped: they decompose
			// into multiple series (_bucket, _sum, _count) that require special
			// handling. Only scalar metric types (counter, gauge, untyped) are
			// forwarded as-is.
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
