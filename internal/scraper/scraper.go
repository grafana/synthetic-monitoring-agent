package scraper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	kitlog "github.com/go-kit/kit/log" //nolint:staticcheck // TODO(mem): replace in BBE
	"github.com/go-kit/kit/log/level"  //nolint:staticcheck // TODO(mem): replace in BBE
	"github.com/go-logfmt/logfmt"
	"github.com/mmcloughlin/geohash"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	prom "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/logproto"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

const (
	ProbeSuccessMetricName = "probe_success"
	CheckInfoMetricName    = "sm_check_info"
	CheckInfoSource        = "synthetic-monitoring-agent"
	maxLabelValueLength    = 2048 // this is the default value in Prometheus
)

var (
	staleNaN    uint64  = 0x7ff0000000000002
	staleMarker float64 = math.Float64frombits(staleNaN)
)

type Incrementer interface {
	Inc()
}

type IncrementerVec interface {
	WithLabelValues(...string) Incrementer
}

type counterVecWrapper struct {
	c *prometheus.CounterVec
}

func (c *counterVecWrapper) WithLabelValues(v ...string) Incrementer {
	return c.c.WithLabelValues(v...)
}

func NewIncrementerFromCounterVec(c *prometheus.CounterVec) IncrementerVec {
	return &counterVecWrapper{c: c}
}

type Scraper struct {
	publisher     pusher.Publisher
	cancel        context.CancelFunc
	checkName     string
	target        string
	logger        zerolog.Logger
	check         model.Check
	probe         sm.Probe
	prober        prober.Prober
	stop          chan struct{}
	scrapeCounter Incrementer
	errorCounter  IncrementerVec
	summaries     map[uint64]prometheus.Summary
	histograms    map[uint64]prometheus.Histogram
}

type TimeSeries = []prompb.TimeSeries
type Streams = []logproto.Stream

type probeData struct {
	tenantId model.GlobalID
	ts       TimeSeries
	streams  Streams
}

func (d *probeData) Metrics() TimeSeries {
	return d.ts
}

func (d *probeData) Streams() Streams {
	return d.streams
}

func (d *probeData) Tenant() model.GlobalID {
	return d.tenantId
}

func New(ctx context.Context, check model.Check, publisher pusher.Publisher, probe sm.Probe, logger zerolog.Logger, scrapeCounter Incrementer, errorCounter IncrementerVec, k6runner k6runner.Runner) (*Scraper, error) {
	return NewWithOpts(ctx, check, ScraperOpts{
		Probe:         probe,
		Publisher:     publisher,
		Logger:        logger,
		ScrapeCounter: scrapeCounter,
		ErrorCounter:  errorCounter,
		ProbeFactory:  prober.NewProberFactory(k6runner),
	})
}

type ScraperOpts struct {
	Probe         sm.Probe
	Publisher     pusher.Publisher
	Logger        zerolog.Logger
	ScrapeCounter Incrementer
	ErrorCounter  IncrementerVec
	ProbeFactory  prober.ProberFactory
}

func NewWithOpts(ctx context.Context, check model.Check, opts ScraperOpts) (*Scraper, error) {
	checkName := check.Type().String()

	logger := opts.Logger.With().
		Int("region_id", check.RegionId).
		Int64("tenantId", check.TenantId).
		Int64("check_id", check.Id).
		Str("probe", opts.Probe.Name).
		Str("target", check.Target).
		Str("job", check.Job).
		Str("check", checkName).
		Logger()

	sctx, cancel := context.WithCancel(ctx)
	smProber, target, err := opts.ProbeFactory.New(sctx, logger, check)
	if err != nil {
		cancel()
		return nil, err
	}

	return &Scraper{
		publisher:     opts.Publisher,
		cancel:        cancel,
		checkName:     checkName,
		target:        target,
		logger:        logger,
		check:         check,
		probe:         opts.Probe,
		prober:        smProber,
		stop:          make(chan struct{}),
		scrapeCounter: opts.ScrapeCounter,
		errorCounter:  opts.ErrorCounter,
		summaries:     make(map[uint64]prometheus.Summary),
		histograms:    make(map[uint64]prometheus.Histogram),
	}, nil
}

var (
	errCheckFailed       = errors.New("probe failed")
	errUnsupportedMetric = errors.New("unsupported metric type")
)

type checkStateMachine struct {
	passes    int
	failures  int
	threshold int
}

func (sm *checkStateMachine) fail(cb func()) {
	wasFailing := sm.isFailing()
	sm.passes = 0
	sm.failures++
	isFailing := sm.isFailing()

	if isFailing != wasFailing {
		cb()
	}
}

func (sm *checkStateMachine) pass(cb func()) {
	wasPassing := sm.isPassing()
	sm.passes++
	sm.failures = 0
	isPassing := sm.isPassing()

	if isPassing != wasPassing {
		cb()
	}
}

func (sm checkStateMachine) isPassing() bool {
	return sm.passes > sm.threshold
}

func (sm checkStateMachine) isFailing() bool {
	return sm.failures > sm.threshold
}

func (s *Scraper) Run(ctx context.Context) {
	s.logger.Info().Msg("starting scraper")

	// TODO(mem): keep count of the number of successive errors and
	// collect logs if threshold is reached.

	var sm checkStateMachine

	// need to keep the most recently published payload for clean up
	var payload *probeData

	scrape := func(ctx context.Context, t time.Time) {
		s.scrapeCounter.Inc()

		var err error
		payload, err = s.collectData(ctx, t)

		switch {
		case errors.Is(err, errCheckFailed):
			s.errorCounter.WithLabelValues("check").Inc()
			sm.fail(func() {
				s.logger.Info().Msg("check entered FAIL state")
			})

		case err != nil:
			s.errorCounter.WithLabelValues("collector").Inc()
			s.logger.Error().Err(err).Msg("error collecting data")
			return

		default:
			sm.pass(func() {
				s.logger.Info().Msg("check entered PASS state")
			})
		}

		if payload != nil {
			s.publisher.Publish(payload)
		}
	}

	cleanup := func(ctx context.Context, t time.Time) {
		if payload == nil {
			return
		}

		staleSample := prompb.Sample{
			Timestamp: t.UnixNano()/1e6 + 1, // ms
			Value:     staleMarker,
		}

		for i := range payload.ts {
			ts := &payload.ts[i]
			for j := range ts.Samples {
				ts.Samples[j] = staleSample
			}
		}

		payload.streams = nil

		s.publisher.Publish(payload)

		payload = nil
	}

	offset := s.check.Offset
	if offset == 0 {
		offset = rand.Int63n(s.check.Frequency)
	}

	tickWithOffset(ctx, s.stop, scrape, cleanup, offset, s.check.Frequency)

	s.cancel()

	s.logger.Info().Msg("scraper stopped")
}

func (s *Scraper) Stop() {
	s.logger.Info().Msg("stopping scraper")
	close(s.stop)
}

func (s Scraper) CheckType() sm.CheckType {
	return s.check.Type()
}

func (s Scraper) ConfigVersion() string {
	return s.check.ConfigVersion()
}

func (s Scraper) LastModified() float64 {
	return s.check.Modified
}

func tickWithOffset(ctx context.Context, stop <-chan struct{}, f func(context.Context, time.Time), cleanup func(context.Context, time.Time), offset, period int64) {
	timer := time.NewTimer(time.Duration(offset) * time.Millisecond)

	var lastTick time.Time

	select {
	case <-ctx.Done():
		if !timer.Stop() {
			<-timer.C
		}
		return

	case <-stop:
		if !timer.Stop() {
			<-timer.C
		}
		// we haven't done anything yet, no clean up
		return

	case t := <-timer.C:
		lastTick = t
		f(ctx, t)
	}

	ticker := time.NewTicker(time.Duration(period) * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return

		case <-stop:
			ticker.Stop()
			// if we are here, we already pushed something
			// at least once, lastTick cannot be zero, but
			// just in case...
			if !lastTick.IsZero() {
				cleanup(ctx, lastTick)
			}
			return

		case t := <-ticker.C:
			lastTick = t
			f(ctx, t)
		}
	}
}

func (s Scraper) collectData(ctx context.Context, t time.Time) (*probeData, error) {
	target := s.target

	// These are the labels defined by the user.
	userLabels := s.buildUserLabels()

	// These labels are applied to the sm_check_info metric.
	checkInfoLabels := s.buildCheckInfoLabels(userLabels)

	if len(checkInfoLabels) > sm.MaxMetricLabels {
		// This should never happen.
		return nil, fmt.Errorf("invalid configuration, too many labels: %d", len(checkInfoLabels))
	}

	// GrafanaCloud loki limits log entries to 15 labels.
	// 7 labels are needed here, leaving 8 labels for users to split between to checks and probes.
	logLabels := []labelPair{
		{name: "probe", value: s.probe.Name},
		{name: "region", value: s.probe.Region},
		{name: "instance", value: s.check.Target},
		{name: "job", value: s.check.Job},
		{name: "check_name", value: s.checkName},
		{name: "source", value: CheckInfoSource}, // identify log lines that belong to synthetic-monitoring-agent
	}
	logLabels = append(logLabels, userLabels...)

	// set up logger to capture check logs
	logs := bytes.Buffer{}
	bl := kitlog.NewLogfmtLogger(&logs)

	// set up logger to capture all the labels as part of the log entry
	loggerLabels := make([]interface{}, 0, 2*(2+len(logLabels)))
	loggerLabels = append(loggerLabels, "ts", kitlog.DefaultTimestampUTC, "target", target)
	for _, l := range logLabels {
		loggerLabels = append(loggerLabels, l.name, l.value)
	}

	sl := kitlog.With(bl, loggerLabels...)

	success, mfs, err := getProbeMetrics(
		ctx,
		s.prober,
		target,
		time.Duration(s.check.Timeout)*time.Millisecond,
		checkInfoLabels,
		s.summaries, s.histograms,
		sl,
		s.check.BasicMetricsOnly,
	)
	if err != nil {
		return nil, err
	}

	// TODO(mem): this is constant for the scraper, move this
	// outside this function?
	metricLabels := []labelPair{
		{name: "probe", value: s.probe.Name},
		{name: "config_version", value: s.check.ConfigVersion()},
		{name: "instance", value: s.check.Target},
		{name: "job", value: s.check.Job},
		// {name: "source", value: CheckInfoSource}, // identify metrics that belong to synthetic-monitoring-agent
	}

	ts := s.extractTimeseries(t, mfs, metricLabels)

	successValue := "1"
	if !success {
		err = errCheckFailed
		successValue = "0"
	}

	if len(logLabels) >= sm.MaxLogLabels {
		logLabels = logLabels[:sm.MaxLogLabels-1]
	}
	logLabels = append(logLabels, labelPair{name: ProbeSuccessMetricName, value: successValue}) // identify log lines that are failures

	// streams need to have all the labels applied to them because
	// loki does not support joins
	streams := s.extractLogs(t, logs.Bytes(), logLabels)

	return &probeData{ts: ts, streams: streams, tenantId: s.check.GlobalTenantID()}, err
}

func getProbeMetrics(
	ctx context.Context,
	prober prober.Prober,
	target string,
	timeout time.Duration,
	checkInfoLabels map[string]string,
	summaries map[uint64]prometheus.Summary,
	histograms map[uint64]prometheus.Histogram,
	logger kitlog.Logger,
	basicMetricsOnly bool,
) (bool, []*dto.MetricFamily, error) {
	registry := prometheus.NewRegistry()

	success := runProber(ctx, prober, target, timeout, registry, checkInfoLabels, logger)

	mfs, err := registry.Gather()
	if err != nil {
		return success, nil, fmt.Errorf(`extracting data from blackbox-exporter: %w`, err)
	}

	registry = prometheus.NewRegistry()

	if err := getDerivedMetrics(mfs, summaries, histograms, registry, basicMetricsOnly); err != nil {
		return success, nil, fmt.Errorf(`getting derived metrics: %w`, err)
	}

	dmfs, err := registry.Gather()
	if err != nil {
		return success, nil, fmt.Errorf(`extracting derived metrics: %w`, err)
	}

	mfs = append(mfs, dmfs...)

	return success, mfs, nil
}

func runProber(
	ctx context.Context,
	prober prober.Prober,
	target string,
	timeout time.Duration,
	registry *prometheus.Registry,
	checkInfoLabels map[string]string,
	logger kitlog.Logger,
) bool {
	start := time.Now()

	_ = level.Info(logger).Log("msg", "Beginning check", "type", prober.Name(), "timeout_seconds", timeout.Seconds())

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	success := prober.Probe(checkCtx, target, registry, logger)

	duration := time.Since(start).Seconds()

	probeSuccessGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: ProbeSuccessMetricName,
		Help: "Displays whether or not the probe was a success",
	})
	probeDurationGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_duration_seconds",
		Help: "Returns how long the probe took to complete in seconds",
	})
	smCheckInfo := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        CheckInfoMetricName,
		Help:        "Provides information about a single check configuration",
		ConstLabels: checkInfoLabels,
	})

	registry.MustRegister(probeSuccessGauge)
	registry.MustRegister(probeDurationGauge)
	registry.MustRegister(smCheckInfo)

	probeDurationGauge.Set(duration)

	if success {
		probeSuccessGauge.Set(1)
		_ = level.Info(logger).Log("msg", "Check succeeded", "duration_seconds", duration)
	} else {
		probeSuccessGauge.Set(0)
		_ = level.Error(logger).Log("msg", "Check failed", "duration_seconds", duration)
	}

	smCheckInfo.Set(1)

	return success
}

func getDerivedMetrics(mfs []*dto.MetricFamily, summaries map[uint64]prometheus.Summary, histograms map[uint64]prometheus.Histogram, registry *prometheus.Registry, basicMetricsOnly bool) error {
	for _, mf := range mfs {
		switch {
		case mf.GetType() == dto.MetricType_GAUGE && mf.GetName() == ProbeSuccessMetricName:
			derivedMetricName := "probe_all_success"

			for _, metric := range mf.GetMetric() {
				_, err := updateSummaryFromMetric(derivedMetricName, mf.GetHelp(), metric, summaries, registry)
				if err != nil {
					return err
				}
			}

		case mf.GetType() == dto.MetricType_GAUGE:
			metricName := mf.GetName()

			// we need to keep probe_all_duration_seconds
			// because we use it to build a more reliable
			// way of computing "uptime".
			if metricName != "probe_duration_seconds" && basicMetricsOnly {
				continue
			}

			suffixes := []string{"_duration_seconds", "_time_seconds"}

			for _, suffix := range suffixes {
				if strings.HasSuffix(metricName, suffix) {
					derivedMetricName := strings.TrimSuffix(metricName, suffix) + "_all" + suffix

					for _, metric := range mf.GetMetric() {
						_, err := updateHistogramFromMetric(derivedMetricName, mf.GetHelp(), metric, histograms, registry)
						if err != nil {
							return err
						}
					}

					break
				}
			}
		}
	}

	return nil
}

func (s Scraper) extractLogs(t time.Time, logs []byte, sharedLabels []labelPair) Streams {
	var line strings.Builder

	dec := logfmt.NewDecoder(bytes.NewReader(logs))

	labels := make([]labelPair, 0, len(sharedLabels))
	var entries []logproto.Entry
RECORD:
	for dec.ScanRecord() {
		var t time.Time

		line.Reset()

		enc := logfmt.NewEncoder(&line)

		labels = labels[:0]
		labels = append(labels, sharedLabels...)

		for dec.ScanKeyval() {
			value := dec.Value()

			switch key := dec.Key(); string(key) {
			case "ts":
				var err error
				t, err = time.Parse(time.RFC3339Nano, string(value))
				if err != nil {
					// We should never hit this as the timestamp string in the log should be valid.
					// Without a timestamp we cannot do anything. And we cannot use something like
					// time.Now() because that would mess up other entries
					s.logger.Warn().Err(err).Bytes("value", value).Msg("invalid timestamp scanning logs")
					continue RECORD
				}

			default:
				if err := enc.EncodeKeyval(key, value); err != nil {
					// We should never hit this because all the entries are valid.
					s.logger.Warn().Err(err).Bytes("key", key).Bytes("value", value).Msg("invalid entry scanning logs")
					continue RECORD
				}
			}
		}

		if err := enc.EndRecord(); err != nil {
			s.logger.Warn().Err(err).Msg("encoding logs")
		}
		entries = append(entries, logproto.Entry{
			Timestamp: t,
			Line:      line.String(),
		})
	}

	if err := dec.Err(); err != nil {
		s.logger.Error().Err(err).Msg("decoding logs")
	}

	return Streams{
		logproto.Stream{
			Labels:  fmtLabels(labels),
			Entries: entries,
		},
	}
}

func (s Scraper) extractTimeseries(t time.Time, metrics []*dto.MetricFamily, sharedLabels []labelPair) TimeSeries {
	return extractTimeseries(t, metrics, sharedLabels, s.summaries, s.histograms, s.logger)
}

func extractTimeseries(t time.Time, metrics []*dto.MetricFamily, sharedLabels []labelPair, summaries map[uint64]prometheus.Summary, histograms map[uint64]prometheus.Histogram, logger zerolog.Logger) TimeSeries {
	metricLabels := make([]prompb.Label, 0, len(sharedLabels))
	for _, label := range sharedLabels {
		metricLabels = append(metricLabels, prompb.Label{Name: label.name, Value: truncateLabelValue(label.value)})
	}

	var ts []prompb.TimeSeries
	for _, mf := range metrics {
		mName := mf.GetName()
		mType := mf.GetType()

		for _, m := range mf.GetMetric() {
			ts = appendDtoToTimeseries(ts, t, mName, metricLabels, mType, m)
		}
	}

	return ts
}

func (s Scraper) buildCheckInfoLabels(userLabels []labelPair) map[string]string {
	labels := map[string]string{
		"check_name": s.checkName,
		"region":     s.probe.Region,
		"frequency":  strconv.FormatInt(s.check.Frequency, 10),
		"geohash":    geohash.Encode(float64(s.probe.Latitude), float64(s.probe.Longitude)),
	}
	if s.check.AlertSensitivity != "" && s.check.AlertSensitivity != "none" {
		labels["alert_sensitivity"] = s.check.AlertSensitivity
	}
	for _, label := range userLabels {
		labels[label.name] = label.value
	}
	return labels
}

func (s Scraper) buildUserLabels() []labelPair {
	labels := []labelPair{}
	idx := make(map[string]int)

	// add probe labels
	for _, l := range s.probe.Labels {
		idx[l.Name] = len(labels)

		labels = append(labels,
			labelPair{name: "label_" + l.Name, value: l.Value})
	}

	// add check labels
	for _, l := range s.check.Labels {
		if where, found := idx[l.Name]; found {
			// already there, update value
			labels[where].value = l.Value
			continue
		}

		idx[l.Name] = len(labels)

		labels = append(labels,
			labelPair{name: "label_" + l.Name, value: l.Value})
	}

	return labels
}

func makeTimeseries(t time.Time, value float64, labels ...prompb.Label) prompb.TimeSeries {
	var ts prompb.TimeSeries

	ts.Labels = make([]prompb.Label, len(labels))
	copy(ts.Labels, labels)

	ts.Samples = []prompb.Sample{
		{Timestamp: t.UnixNano() / 1e6, Value: value},
	}

	return ts
}

func appendDtoToTimeseries(ts []prompb.TimeSeries, t time.Time, mName string, sharedLabels []prompb.Label, mType dto.MetricType, metric *dto.Metric) []prompb.TimeSeries {
	ml := metric.GetLabel()

	labels := make([]prompb.Label, 0, 1+len(sharedLabels)+len(ml))
	labels = append(labels, prompb.Label{Name: "__name__", Value: mName})
	labels = append(labels, sharedLabels...)
	for _, l := range ml {
		labels = append(labels, prompb.Label{Name: *(l.Name), Value: truncateLabelValue(*(l.Value))})
	}

	switch mType {
	case dto.MetricType_COUNTER:
		if v := metric.GetCounter(); v != nil && v.Value != nil {
			ts = append(ts, makeTimeseries(t, *v.Value, labels...))
		}

	case dto.MetricType_GAUGE:
		if v := metric.GetGauge(); v != nil && v.Value != nil {
			ts = append(ts, makeTimeseries(t, *v.Value, labels...))
		}

	case dto.MetricType_UNTYPED:
		if v := metric.GetUntyped(); v != nil && v.Value != nil {
			ts = append(ts, makeTimeseries(t, *v.Value, labels...))
		}

	case dto.MetricType_SUMMARY:
		if s := metric.GetSummary(); s != nil {
			sLabels := make([]prompb.Label, len(labels))
			copy(sLabels, labels)

			sLabels[0] = prompb.Label{Name: "__name__", Value: mName + "_sum"}
			ts = append(ts, makeTimeseries(t, s.GetSampleSum(), sLabels...))

			sLabels[0] = prompb.Label{Name: "__name__", Value: mName + "_count"}
			ts = append(ts, makeTimeseries(t, float64(s.GetSampleCount()), sLabels...))

			sLabels = make([]prompb.Label, len(labels)+1)
			copy(sLabels, labels)

			for _, v := range s.GetQuantile() {
				sLabels[len(sLabels)-1] = prompb.Label{
					Name:  "quantile",
					Value: strconv.FormatFloat(v.GetQuantile(), 'G', -1, 64),
				}
				ts = append(ts, makeTimeseries(t, v.GetValue(), sLabels...))
			}
		}

	case dto.MetricType_HISTOGRAM:
		if h := metric.GetHistogram(); h != nil {
			if b := h.GetBucket(); b != nil {
				hLabels := make([]prompb.Label, len(labels))
				copy(hLabels, labels)

				hLabels[0] = prompb.Label{Name: "__name__", Value: mName + "_sum"}
				ts = append(ts, makeTimeseries(t, h.GetSampleSum(), hLabels...))

				hLabels[0] = prompb.Label{Name: "__name__", Value: mName + "_count"}
				ts = append(ts, makeTimeseries(t, float64(h.GetSampleCount()), hLabels...))

				hLabels = make([]prompb.Label, len(labels)+1)
				copy(hLabels, labels)

				hLabels[0] = prompb.Label{Name: "__name__", Value: mName + "_bucket"}
				for _, v := range b {
					hLabels[len(hLabels)-1] = prompb.Label{
						Name:  "le",
						Value: strconv.FormatFloat(v.GetUpperBound(), 'G', -1, 64),
					}
					ts = append(ts, makeTimeseries(t, float64(v.GetCumulativeCount()), hLabels...))
				}

				// Add the +Inf bucket, which corresponds to the sample count.
				hLabels[len(hLabels)-1] = prompb.Label{
					Name:  "le",
					Value: "+Inf",
				}
				ts = append(ts, makeTimeseries(t, float64(h.GetSampleCount()), hLabels...))
			}
		}
	}

	return ts
}

type labelPair struct {
	name  string
	value string
}

func fmtLabels(labels []labelPair) string {
	if len(labels) == 0 {
		return ""
	}

	var s strings.Builder

	// these calls do not produce errors, the errors are required to
	// satisfy interfaces
	_, _ = s.WriteRune('{')

	for i, pair := range labels {
		if i > 0 {
			_, _ = s.WriteRune(',')
		}
		_, _ = s.WriteString(pair.name)
		_, _ = s.WriteRune('=')
		_, _ = s.WriteRune('"')
		_, _ = s.WriteString(pair.value)
		_, _ = s.WriteRune('"')
	}

	_, _ = s.WriteRune('}')

	return s.String()
}

func updateSummaryFromMetric(mName, help string, m *dto.Metric, summaries map[uint64]prometheus.Summary, registry *prometheus.Registry) (prometheus.Summary, error) {
	var value float64

	switch {
	case m.GetCounter() != nil:
		value = m.GetCounter().GetValue()

	case m.GetGauge() != nil:
		value = m.GetGauge().GetValue()

	default:
		return nil, errUnsupportedMetric
	}

	mHash := hashMetricNameAndLabels(mName, m.GetLabel())

	summary, found := summaries[mHash]
	if !found {
		summary = prometheus.NewSummary(prometheus.SummaryOpts{
			Name:        mName,
			Help:        help + " (summary)",
			ConstLabels: getLabels(m),
		})

		summaries[mHash] = summary
	}

	if err := registry.Register(summary); err != nil {
		return nil, err
	}

	summary.Observe(value)

	return summary, nil
}

func updateHistogramFromMetric(mName, help string, m *dto.Metric, histograms map[uint64]prometheus.Histogram, registry *prometheus.Registry) (prometheus.Histogram, error) {
	var value float64

	switch {
	case m.GetCounter() != nil:
		value = m.GetCounter().GetValue()

	case m.GetGauge() != nil:
		value = m.GetGauge().GetValue()

	default:
		return nil, errUnsupportedMetric
	}

	mHash := hashMetricNameAndLabels(mName, m.GetLabel())

	histogram, found := histograms[mHash]
	if !found {
		histogram = prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:        mName,
			Help:        help + " (histogram)",
			ConstLabels: getLabels(m),
			Buckets:     prometheus.DefBuckets,
		})

		histograms[mHash] = histogram
	}

	if err := registry.Register(histogram); err != nil {
		return nil, err
	}

	histogram.Observe(value)

	return histogram, nil
}

func hashMetricNameAndLabels(name string, dtoLabels []*dto.LabelPair) uint64 {
	ls := prom.LabelSet{
		prom.MetricNameLabel: prom.LabelValue(name),
	}

	for _, label := range dtoLabels {
		ls[prom.LabelName(label.GetName())] = prom.LabelValue(label.GetValue())
	}

	return uint64(ls.Fingerprint())
}

func getLabels(m *dto.Metric) map[string]string {
	if len(m.GetLabel()) == 0 {
		return nil
	}

	labels := make(map[string]string)

	for _, label := range m.GetLabel() {
		labels[label.GetName()] = label.GetValue()
	}

	return labels
}

func truncateLabelValue(str string) string {
	if len(str) > maxLabelValueLength {
		b := []byte(str[:maxLabelValueLength])
		for i := maxLabelValueLength - 3; i < maxLabelValueLength; i++ {
			b[i] = '.'
		}
		str = string(b)
	}

	return str
}
