package scraper

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-logfmt/logfmt"
	"github.com/grafana/loki/pkg/logproto"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pusher"
	"github.com/grafana/worldping-blackbox-sidecar/pkg/pb/worldping"
	"github.com/mmcloughlin/geohash"
	bbeconfig "github.com/prometheus/blackbox_exporter/config"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
)

var (
	staleNaN    uint64  = 0x7ff0000000000002
	staleMarker float64 = math.Float64frombits(staleNaN)
)

const (
	ScraperTypeDNS  = "dns"
	ScraperTypeHTTP = "http"
	ScraperTypePing = "ping"
	ScraperTypeTcp  = "tcp"
)

type Scraper struct {
	publishCh     chan<- pusher.Payload
	cancel        context.CancelFunc
	checkName     string
	provider      url.URL // provider is the BBE URL
	logger        zerolog.Logger
	check         worldping.Check
	probe         worldping.Probe
	bbeModule     *bbeconfig.Module
	moduleName    string
	stop          chan struct{}
	scrapeCounter prometheus.Counter
	errorCounter  *prometheus.CounterVec
	summaries     map[uint64]prometheus.Summary
	histograms    map[uint64]prometheus.Histogram
}

type TimeSeries = []prompb.TimeSeries
type Streams = []logproto.Stream

type probeData struct {
	tenantId int64
	ts       TimeSeries
	streams  Streams
}

func (d *probeData) Metrics() TimeSeries {
	return d.ts
}

func (d *probeData) Streams() Streams {
	return d.streams
}

func (d *probeData) Tenant() int64 {
	return d.tenantId
}

func New(ctx context.Context, check worldping.Check, publishCh chan<- pusher.Payload, probe worldping.Probe, probeURL url.URL, logger zerolog.Logger, scrapeCounter prometheus.Counter, errorCounter *prometheus.CounterVec) (*Scraper, error) {
	logger = logger.With().
		Int64("check_id", check.Id).
		Str("probe", probe.Name).
		Str("provider", probeURL.String()).
		Str("target", check.Target).
		Str("job", check.Job).
		Logger()

	sctx, cancel := context.WithCancel(ctx)
	checkName, bbeModule, target, err := mapSettings(sctx, logger, check.Target, check.Settings)
	if err != nil {
		cancel()
		return nil, err
	}

	moduleName := fmt.Sprintf("check_%d", check.Id)

	bbeModule.Timeout = time.Duration(check.Timeout) * time.Millisecond

	q := probeURL.Query()
	q.Set("target", target)
	q.Set("module", moduleName)
	probeURL.RawQuery = q.Encode()

	return &Scraper{
		publishCh:     publishCh,
		cancel:        cancel,
		checkName:     checkName,
		provider:      probeURL,
		logger:        logger.With().Str("check", checkName).Logger(),
		check:         check,
		probe:         probe,
		bbeModule:     &bbeModule,
		moduleName:    moduleName,
		stop:          make(chan struct{}),
		scrapeCounter: scrapeCounter,
		errorCounter:  errorCounter,
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
			s.publishCh <- payload
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

		s.publishCh <- payload

		payload = nil
	}

	tickWithOffset(ctx, s.stop, scrape, cleanup, s.check.Offset, s.check.Frequency)

	s.cancel()

	s.logger.Info().Msg("scraper stopped")
}

func (s *Scraper) Stop() {
	s.logger.Info().Msg("stopping scraper")
	close(s.stop)
}

func (s Scraper) CheckType() string {
	// XXX(mem): this shouldn't be here, it should be in
	// worldping.Check

	switch {
	case s.check.Settings.Dns != nil:
		return ScraperTypeDNS

	case s.check.Settings.Http != nil:
		return ScraperTypeHTTP

	case s.check.Settings.Ping != nil:
		return ScraperTypePing

	case s.check.Settings.Tcp != nil:
		return ScraperTypeTcp
	}

	// we need this to make sure that adding a check type does not
	// go unnoticed in here
	panic("unknown check type")
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

func (s *Scraper) GetModuleName() string {
	return s.moduleName
}

func (s *Scraper) GetModuleConfig() interface{} {
	return s.bbeModule
}

func (s Scraper) collectData(ctx context.Context, t time.Time) (*probeData, error) {
	u := s.provider
	q := u.Query()

	// this is needed in order to obtain the logs alongside the metrics
	q.Add("debug", "true")

	// XXX(mem): at this point, we shouldn't care about the check
	// type, as we have already set everything up to just hit BBE,
	// but this is special-casing HTTP because we need to modify the
	// target to append a cache-busting parameter that includes the
	// current timestamp.
	if s.CheckType() == ScraperTypeHTTP && s.check.Settings.Http.CacheBustingQueryParamName != "" {
		q.Set("target", addCacheBustParam(s.check.Target, s.check.Settings.Http.CacheBustingQueryParamName, s.probe.Name))
	}

	u.RawQuery = q.Encode()
	address := u.String()

	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		err = fmt.Errorf(`msg="creating new request" probe=%s target=%s job=%s address=%s err="%w"`, s.probe.Name, s.check.Target, s.check.Job, address, err)
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		err = fmt.Errorf(`msg="requesting data from" probe=%s target=%s job=%s address=%s err="%w"`, s.probe.Name, s.check.Target, s.check.Job, address, err)
		return nil, err
	}

	defer func() {
		// drain body
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	metrics, logs, err := extractMetricsAndLogs(resp.Body)
	if err != nil {
		err = fmt.Errorf(`msg="extracting data from blackbox-exporter" probe=%s target=%s job=%s address=%s err="%w"`, s.probe.Name, s.check.Target, s.check.Job, address, err)
		return nil, err
	}

	// TODO(mem): this is constant for the scraper, move this
	// outside this function?
	sharedLabels := []labelPair{
		{name: "probe", value: s.probe.Name},
		{name: "config_version", value: strconv.FormatInt(int64(s.check.Modified*1000000000), 10)},
		{name: "instance", value: s.check.Target},
		{name: "job", value: s.check.Job},
	}

	checkInfoLabels := s.buildCheckInfoLabels()

	// timeseries need to differentiate between base labels and
	// check info labels in order to be able to apply the later only
	// to the worldping_check_info metric
	ts, err := s.extractTimeseries(t, metrics, sharedLabels, checkInfoLabels)

	successValue := "1"

	switch {
	case err == nil:
		// OK, value already set

	case errors.Is(err, errCheckFailed):
		// probe failed
		successValue = "0"

	default:
		// something else failed
		return nil, err
	}

	allLabels := append(sharedLabels, checkInfoLabels...)

	allLabels = append(allLabels,
		labelPair{name: "probe_success", value: successValue}, // identify log lines that are failures
		labelPair{name: "source", value: "worldping"})         // identify log lines that belong to worldping

	// streams need to have all the labels applied to them because
	// loki does not support joins
	streams := s.extractLogs(t, logs, allLabels)

	return &probeData{ts: ts, streams: streams, tenantId: s.check.TenantId}, err
}

func (s Scraper) extractLogs(t time.Time, logs []byte, sharedLabels []labelPair) Streams {
	var streams Streams
	var line strings.Builder

	dec := logfmt.NewDecoder(bytes.NewReader(logs))

	labels := make([]labelPair, 0, len(sharedLabels))

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

			case "caller", "module":
				// skip

			case "level":
				// this has to be tranlated to a label
				labels = append(labels, labelPair{name: "level", value: string(value)})

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

		// this is creating one stream per log line because the labels might have to change between lines (level
		// is not going to be the same).
		streams = append(streams, logproto.Stream{
			Labels: fmtLabels(labels),
			Entries: []logproto.Entry{
				{
					Timestamp: t,
					Line:      line.String(),
				},
			},
		})
	}

	if err := dec.Err(); err != nil {
		s.logger.Error().Err(err).Msg("decoding logs")
	}

	return streams
}

func (s Scraper) extractTimeseries(t time.Time, metrics []byte, sharedLabels, checkInfoLabels []labelPair) (TimeSeries, error) {
	// XXX(mem): the following is needed in order to derive the
	// correct format from the response headers, but since we are
	// passing debug=true, we loose access to that.
	//
	// format := expfmt.ResponseFormat(resp.Header)
	//
	// Instead hard-code the format to be plain text.
	format := expfmt.FmtText

	dec := expfmt.NewDecoder(bytes.NewReader(metrics), format)

	metricLabels := make([]prompb.Label, 0, len(sharedLabels))
	for _, label := range sharedLabels {
		metricLabels = append(metricLabels, prompb.Label{Name: label.name, Value: label.value})
	}

	checkInfoMetric := makeCheckInfoMetrics(t, sharedLabels, checkInfoLabels)

	ts := []prompb.TimeSeries{checkInfoMetric}
	var checkErr error
	for {
		var mf dto.MetricFamily

		switch err := dec.Decode(&mf); err {
		case nil:
			// got metrics
			mName := mf.GetName()
			mType := mf.GetType()

			for _, m := range mf.GetMetric() {
				ts = appendDtoToTimeseries(ts, t, mName, metricLabels, mType, m)

				switch {
				case mName == "probe_success" && mType == dto.MetricType_GAUGE:
					if m.GetGauge().GetValue() == 0 {
						// return an error to the caller, signalling that the check failed.
						s.logger.Info().Err(errCheckFailed).Msg("check failed")
						checkErr = errCheckFailed
					}

					// derive a summary from this metric
					dName, dm, err := updateSummaryFromMetric(mName, m, s.summaries)
					if err != nil {
						s.logger.Warn().Err(err).Str("name", mName).Interface("metric", m).Msg("updating summary from metric")
					} else {
						ts = appendDtoToTimeseries(ts, t, dName, metricLabels, dto.MetricType_SUMMARY, dm)
					}

				case mType == dto.MetricType_GAUGE && strings.HasSuffix(mName, "_seconds"):
					// derive a histogram from this metric
					dName, dm, err := updateHistogramFromMetric(mName, m, s.histograms)
					if err != nil {
						s.logger.Warn().Err(err).Str("name", mName).Interface("metric", m).Msg("updating histogram from metric")
					} else {
						ts = appendDtoToTimeseries(ts, t, dName, metricLabels, dto.MetricType_HISTOGRAM, dm)
					}
				}
			}

		case io.EOF:
			return ts, checkErr

		default:
			s.logger.Error().Err(err).Msg("decoding results from blackbox-exporter")
			return nil, err
		}
	}
}

func (s Scraper) buildCheckInfoLabels() []labelPair {
	labels := []labelPair{
		{name: "check_name", value: s.checkName},
		{name: "frequency", value: strconv.FormatInt(s.check.Frequency, 10)},
		{name: "latitude", value: strconv.FormatFloat(float64(s.probe.Latitude), 'f', 6, 32)},
		{name: "longitude", value: strconv.FormatFloat(float64(s.probe.Longitude), 'f', 6, 32)},
		{name: "geohash", value: geohash.Encode(float64(s.probe.Latitude), float64(s.probe.Longitude))},
	}

	seen := make(map[string]struct{})

	// add check labels
	for _, l := range s.check.Labels {
		seen[l.Name] = struct{}{}

		labels = append(labels,
			labelPair{name: "label_" + l.Name, value: l.Value})
	}

	// add probe labels
	for _, l := range s.probe.Labels {
		if _, found := seen[l.Name]; found {
			// checks can override probe labels
			continue
		}

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
		labels = append(labels, prompb.Label{Name: *(l.Name), Value: *(l.Value)})
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
			}
		}
	}

	return ts
}

func makeCheckInfoMetrics(t time.Time, sharedLabels, checkInfoLabels []labelPair) prompb.TimeSeries {
	labels := make([]prompb.Label, 0, 1+len(sharedLabels)+len(checkInfoLabels))

	labels = append(labels, prompb.Label{Name: "__name__", Value: "worldping_check_info"})

	for _, label := range sharedLabels {
		labels = append(labels, prompb.Label{Name: label.name, Value: label.value})
	}

	for _, label := range checkInfoLabels {
		labels = append(labels, prompb.Label{Name: label.name, Value: label.value})
	}

	return makeTimeseries(t, 1, labels...)
}

func extractMetricsAndLogs(r io.Reader) ([]byte, []byte, error) {
	type extractorState int

	const (
		stateLookingForHeader extractorState = iota
		stateInLogs
		stateInMetrics
	)

	var (
		state   extractorState
		metrics bytes.Buffer
		logs    bytes.Buffer
		cur     *bytes.Buffer
	)

	s := bufio.NewScanner(r)

SCAN:
	for s.Scan() {
		switch state {
		case stateLookingForHeader:
			switch text := s.Text(); text {
			case "Logs for the probe:":
				state = stateInLogs
				cur = &logs

			case "Metrics that would have been returned:":
				state = stateInMetrics
				cur = &metrics
			}

		case stateInLogs, stateInMetrics:
			// first blank line ends the data and goes back
			// to searching for the next header
			if s.Text() == "" {
				// we break out early if we have both
				// logs and metrics
				if logs.Len() > 0 && metrics.Len() > 0 {
					break SCAN
				}
				state = stateLookingForHeader
				continue
			}

			if _, err := cur.Write(s.Bytes()); err != nil {
				return nil, nil, err
			}

			if _, err := cur.WriteRune('\n'); err != nil {
				return nil, nil, err
			}
		}
	}

	if err := s.Err(); err != nil {
		return nil, nil, err
	}

	return metrics.Bytes(), logs.Bytes(), nil
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

func mapSettings(ctx context.Context, logger zerolog.Logger, target string, settings worldping.CheckSettings) (string, bbeconfig.Module, string, error) {
	// Map the change to a blackbox exporter module
	switch {
	case settings.Ping != nil:
		return "ping", pingSettingsToBBEModule(settings.Ping), target, nil

	case settings.Http != nil:
		m, err := httpSettingsToBBEModule(ctx, logger, settings.Http)
		return "http", m, target, err

	case settings.Dns != nil:
		return "dns", dnsSettingsToBBEModule(ctx, settings.Dns, target), settings.Dns.Server, nil

	case settings.Tcp != nil:
		m, err := tcpSettingsToBBEModule(ctx, logger, settings.Tcp)
		return "tcp", m, target, err

	default:
		return "", bbeconfig.Module{}, "", fmt.Errorf("unsupported change")
	}
}

func ipVersionToIpProtocol(v worldping.IpVersion) (string, bool) {
	switch v {
	case worldping.IpVersion_V4:
		// preferred_ip_protocol = ip4
		// ip_protocol_fallback = false
		return "ip4", false
	case worldping.IpVersion_V6:
		// preferred_ip_protocol = ip6
		// ip_protocol_fallback = false
		return "ip6", false
	case worldping.IpVersion_Any:
		// preferred_ip_protocol = ip6
		// ip_protocol_fallback = true
		return "ip6", true
	}

	return "", false
}

func pingSettingsToBBEModule(settings *worldping.PingSettings) bbeconfig.Module {
	var m bbeconfig.Module

	m.Prober = "icmp"

	m.ICMP.IPProtocol, m.ICMP.IPProtocolFallback = ipVersionToIpProtocol(settings.IpVersion)

	m.ICMP.SourceIPAddress = settings.SourceIpAddress

	m.ICMP.PayloadSize = int(settings.PayloadSize)

	m.ICMP.DontFragment = settings.DontFragment

	return m
}

func httpSettingsToBBEModule(ctx context.Context, logger zerolog.Logger, settings *worldping.HttpSettings) (bbeconfig.Module, error) {
	var m bbeconfig.Module

	m.Prober = "http"

	m.HTTP.IPProtocol, m.HTTP.IPProtocolFallback = ipVersionToIpProtocol(settings.IpVersion)

	m.HTTP.Body = settings.Body

	m.HTTP.Method = settings.Method.String()

	m.HTTP.FailIfSSL = settings.FailIfSSL

	m.HTTP.FailIfNotSSL = settings.FailIfNotSSL

	m.HTTP.NoFollowRedirects = settings.NoFollowRedirects

	if len(settings.Headers) > 0 {
		m.HTTP.Headers = make(map[string]string)
	}
	for _, header := range settings.Headers {
		parts := strings.SplitN(header, ":", 2)
		var value string
		if len(parts) == 2 {
			value = strings.TrimLeft(parts[1], " ")
		}
		m.HTTP.Headers[parts[0]] = value
	}

	m.HTTP.ValidStatusCodes = make([]int, 0, len(settings.ValidStatusCodes))
	for _, code := range settings.ValidStatusCodes {
		m.HTTP.ValidStatusCodes = append(m.HTTP.ValidStatusCodes, int(code))
	}

	m.HTTP.ValidHTTPVersions = make([]string, len(settings.ValidHTTPVersions))
	copy(m.HTTP.ValidHTTPVersions, settings.ValidHTTPVersions)

	m.HTTP.FailIfBodyMatchesRegexp = make([]string, len(settings.FailIfBodyMatchesRegexp))
	copy(m.HTTP.FailIfBodyMatchesRegexp, settings.FailIfBodyMatchesRegexp)

	m.HTTP.FailIfBodyNotMatchesRegexp = make([]string, len(settings.FailIfBodyNotMatchesRegexp))
	copy(m.HTTP.FailIfBodyNotMatchesRegexp, settings.FailIfBodyNotMatchesRegexp)

	m.HTTP.FailIfHeaderMatchesRegexp = make([]bbeconfig.HeaderMatch, 0, len(settings.FailIfHeaderMatchesRegexp))
	for _, match := range settings.FailIfHeaderMatchesRegexp {
		m.HTTP.FailIfHeaderMatchesRegexp = append(m.HTTP.FailIfHeaderMatchesRegexp, bbeconfig.HeaderMatch{
			Header:       match.Header,
			Regexp:       match.Regexp,
			AllowMissing: match.AllowMissing,
		})
	}

	m.HTTP.FailIfHeaderNotMatchesRegexp = make([]bbeconfig.HeaderMatch, 0, len(settings.FailIfHeaderNotMatchesRegexp))
	for _, match := range settings.FailIfHeaderNotMatchesRegexp {
		m.HTTP.FailIfHeaderNotMatchesRegexp = append(m.HTTP.FailIfHeaderNotMatchesRegexp, bbeconfig.HeaderMatch{
			Header:       match.Header,
			Regexp:       match.Regexp,
			AllowMissing: match.AllowMissing,
		})
	}

	if settings.TlsConfig != nil {
		var err error
		m.HTTP.HTTPClientConfig.TLSConfig, err = worldpingTLSConfigToBBE(ctx, logger.With().Str("prober", m.Prober).Logger(), settings.TlsConfig)
		if err != nil {
			return m, err
		}
	}

	return m, nil
}

func dnsSettingsToBBEModule(ctx context.Context, settings *worldping.DnsSettings, target string) bbeconfig.Module {
	var m bbeconfig.Module

	m.Prober = "dns"
	m.DNS.IPProtocol, m.DNS.IPProtocolFallback = ipVersionToIpProtocol(settings.IpVersion)

	// BBE dns_probe actually tests the DNS server, so we
	// need to pass the query (e.g. www.grafana.com) as part
	// of the configuration and the server as the target
	// parameter.
	m.DNS.QueryName = target
	m.DNS.QueryType = settings.RecordType.String()
	m.DNS.SourceIPAddress = settings.SourceIpAddress
	// In the protobuffer definition the protocol is either
	// "TCP" or "UDP", but blackbox-exporter wants "tcp" or
	// "udp".
	m.DNS.TransportProtocol = strings.ToLower(settings.Protocol.String())

	m.DNS.ValidRcodes = settings.ValidRCodes

	if settings.ValidateAnswer != nil {
		m.DNS.ValidateAnswer.FailIfMatchesRegexp = settings.ValidateAnswer.FailIfMatchesRegexp
		m.DNS.ValidateAnswer.FailIfNotMatchesRegexp = settings.ValidateAnswer.FailIfNotMatchesRegexp
	}

	if settings.ValidateAuthority != nil {
		m.DNS.ValidateAuthority.FailIfMatchesRegexp = settings.ValidateAuthority.FailIfMatchesRegexp
		m.DNS.ValidateAuthority.FailIfNotMatchesRegexp = settings.ValidateAuthority.FailIfNotMatchesRegexp
	}

	if settings.ValidateAdditional != nil {
		m.DNS.ValidateAdditional.FailIfMatchesRegexp = settings.ValidateAdditional.FailIfMatchesRegexp
		m.DNS.ValidateAdditional.FailIfNotMatchesRegexp = settings.ValidateAdditional.FailIfNotMatchesRegexp
	}

	return m
}

func tcpSettingsToBBEModule(ctx context.Context, logger zerolog.Logger, settings *worldping.TcpSettings) (bbeconfig.Module, error) {
	var m bbeconfig.Module

	m.Prober = "tcp"
	m.TCP.IPProtocol, m.TCP.IPProtocolFallback = ipVersionToIpProtocol(settings.IpVersion)

	m.TCP.SourceIPAddress = settings.SourceIpAddress

	m.TCP.TLS = settings.Tls

	m.TCP.QueryResponse = make([]bbeconfig.QueryResponse, len(settings.QueryResponse))

	for _, qr := range settings.QueryResponse {
		m.TCP.QueryResponse = append(m.TCP.QueryResponse, bbeconfig.QueryResponse{
			Expect: string(qr.Expect),
			Send:   string(qr.Send),
		})
	}

	if settings.TlsConfig != nil {
		var err error
		m.TCP.TLSConfig, err = worldpingTLSConfigToBBE(ctx, logger.With().Str("prober", m.Prober).Logger(), settings.TlsConfig)
		if err != nil {
			return m, err
		}
	}

	return m, nil
}

func worldpingTLSConfigToBBE(ctx context.Context, logger zerolog.Logger, tlsConfig *worldping.TLSConfig) (promconfig.TLSConfig, error) {
	c := promconfig.TLSConfig{
		InsecureSkipVerify: tlsConfig.InsecureSkipVerify,
		ServerName:         tlsConfig.ServerName,
	}

	if len(tlsConfig.CACert) > 0 {
		fn, err := newDataProvider(ctx, logger, "ca_cert", tlsConfig.CACert)
		if err != nil {
			return promconfig.TLSConfig{}, err
		}
		c.CAFile = fn
	}

	if len(tlsConfig.ClientCert) > 0 {
		fn, err := newDataProvider(ctx, logger, "client_cert", tlsConfig.CACert)
		if err != nil {
			return promconfig.TLSConfig{}, err
		}
		c.CertFile = fn
	}

	if len(tlsConfig.ClientKey) > 0 {
		fn, err := newDataProvider(ctx, logger, "client_key", tlsConfig.CACert)
		if err != nil {
			return promconfig.TLSConfig{}, err
		}
		c.KeyFile = fn
	}

	return c, nil
}

// newDataProvider creates a filesystem object that provides the
// specified data as often as needed. It returns the name under which
// the data can be accessed.
//
// It does NOT try to make guarantees about partial reads. If the reader
// goes away before reaching the end of the data, the next time the
// reader shows up, the writer might continue from the previous
// prosition.
func newDataProvider(ctx context.Context, logger zerolog.Logger, basename string, data []byte) (string, error) {
	fh, err := ioutil.TempFile("", basename+".")
	if err != nil {
		logger.Error().Err(err).Str("basename", basename).Msg("creating temporary file")
		return "", fmt.Errorf("creating temporary file: %w", err)
	}
	defer fh.Close()

	fn := fh.Name()

	if n, err := fh.Write(data); err != nil {
		logger.Error().Err(err).Str("filename", fn).Int("bytes", n).Int("data", len(data)).Msg("writing temporary file")
		return "", fmt.Errorf("writing temporary file for %s: %w", basename, err)
	}

	// play nice and make sure this file gets deleted once the
	// context is cancelled, which could be when the program is
	// shutting down or when the scraper stops.
	go func() {
		<-ctx.Done()
		if err := os.Remove(fn); err != nil {
			logger.Error().Err(err).Str("filename", fn).Msg("removing temporary file")
		}
	}()

	return fn, nil
}

func updateSummaryFromMetric(mName string, m *dto.Metric, summaries map[uint64]prometheus.Summary) (string, *dto.Metric, error) {
	var value float64

	switch {
	case m.GetCounter() != nil:
		value = m.GetCounter().GetValue()

	case m.GetGauge() != nil:
		value = m.GetGauge().GetValue()

	default:
		return "", nil, errUnsupportedMetric
	}

	mHash := hashMetricNameAndLabels(mName, m.GetLabel())

	s, found := summaries[mHash]
	if !found {
		s = prometheus.NewSummary(prometheus.SummaryOpts{
			// since we are not specifying objectives,
			// reusing the name causes only two metrics to
			// be added: name_sum and name_count; if we were
			// to specify objectives here, the same metric
			// name would be used, adding labels to specify
			// the quantiles ("le") and we should transform
			// the metric's name.
			Name:        mName,
			ConstLabels: getLabels(m),
		})
		summaries[mHash] = s
	}

	s.Observe(value)

	dm := new(dto.Metric)

	if err := s.Write(dm); err != nil {
		return "", nil, err
	}

	return mName, dm, nil
}

func updateHistogramFromMetric(mName string, m *dto.Metric, histograms map[uint64]prometheus.Histogram) (string, *dto.Metric, error) {
	var value float64

	switch {
	case m.GetCounter() != nil:
		value = m.GetCounter().GetValue()

	case m.GetGauge() != nil:
		value = m.GetGauge().GetValue()

	default:
		return "", nil, errUnsupportedMetric
	}

	mHash := hashMetricNameAndLabels(mName, m.GetLabel())

	s, found := histograms[mHash]
	if !found {
		s = prometheus.NewHistogram(prometheus.HistogramOpts{
			// reusing the name causes three metrics to be
			// added: name_sum, name_count and name_bucket.
			// The original name is never published as part
			// of the histogram, so it's safe to leave the
			// original name unmodified.
			Name:        mName,
			ConstLabels: getLabels(m),
			Buckets:     prometheus.DefBuckets,
		})
		histograms[mHash] = s
	}

	s.Observe(value)

	dm := new(dto.Metric)

	if err := s.Write(dm); err != nil {
		return "", nil, err
	}

	return mName, dm, nil
}

func hashMetricNameAndLabels(name string, dtoLabels []*dto.LabelPair) uint64 {
	ls := model.LabelSet{
		model.MetricNameLabel: model.LabelValue(name),
	}

	for _, label := range dtoLabels {
		ls[model.LabelName(label.GetName())] = model.LabelValue(label.GetValue())
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

func addCacheBustParam(target, paramName, salt string) string {
	// we already know this URL is valid
	u, _ := url.Parse(target)
	q := u.Query()
	value := hashString(salt, strconv.FormatInt(time.Now().UnixNano(), 10))
	q.Set(paramName, value)
	u.RawQuery = q.Encode()
	return u.String()
}

func hashString(salt, str string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(salt))
	_, _ = h.Write([]byte(str))
	return strconv.FormatUint(h.Sum64(), 16)
}
