package scraper

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/prompb"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// "github.com/prometheus/common/expfmt"

type Scraper struct {
	publishCh chan<- []prompb.TimeSeries
	probeName string
	checkName string
	target    string
	endpoint  string
	logger    logger
}

type logger interface {
	Printf(format string, v ...interface{})
}

type TimeSeries = []prompb.TimeSeries

func New(publishCh chan<- []prompb.TimeSeries, probeName, checkName, target, endpoint string, logger logger) *Scraper {
	return &Scraper{
		publishCh: publishCh,
		probeName: probeName,
		checkName: checkName,
		target:    target,
		endpoint:  endpoint,
		logger:    logger,
	}
}

var errCheckFailed = errors.New("probe failed")

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

func (s Scraper) Run(ctx context.Context) {
	s.logger.Printf("starting scraper at %s for %s", s.probeName, s.target)

	// TODO(mem): keep count of the number of successive errors and
	// collect logs if threshold is reached.

	var sm checkStateMachine

	scrape := func(ctx context.Context, t time.Time) {
		ts, logs, err := s.collectData(ctx, t)

		switch {
		case errors.Is(err, errCheckFailed):
			sm.fail(func() {
				s.logger.Printf(`msg="check entered FAIL state" probe=%s endpoint=%s target=%s`, s.probeName, s.endpoint, s.target)
				// XXX(mem): post logs to Loki
				s.logger.Printf("logs: %s", logs)
			})

		case err != nil:
			s.logger.Printf("Error collecting data from %s: %s", s.target, err)
			return

		default:
			sm.pass(func() {
				s.logger.Printf(`msg="check entered PASS state" probe=%s endpoint=%s target=%s`, s.probeName, s.endpoint, s.target)
			})
		}

		if ts != nil {
			s.publishCh <- ts
		}
	}

	s.logger.Printf("scraping first set")
	scrape(ctx, time.Now())

	const T = 5 * 1000 // period, ms

	ticker := time.NewTicker(T * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():

		case t := <-ticker.C:
			scrape(ctx, t)
		}
	}
}

func (s Scraper) collectData(ctx context.Context, t time.Time) ([]prompb.TimeSeries, []byte, error) {
	u, _ := url.Parse(s.target)
	q := u.Query()
	// this is needed in order to obtain the logs alongside the metrics
	q.Add("debug", "true")
	u.RawQuery = q.Encode()
	target := u.String()

	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("creating new request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("requesting data from %s: %w", target, err)
	}

	defer func() {
		// drain body
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	metrics, logs, err := extractMetricsAndLogs(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("extracting data from blackbox-exporter: %w", err)
	}

	// XXX(mem): the following is needed in order to derive the
	// correct format from the response headers, but since we are
	// passing debug=true, we loose access to that.
	//
	// format := expfmt.ResponseFormat(resp.Header)
	//
	// Instead hard-code the format to be plain text.

	format := expfmt.FmtText

	dec := expfmt.NewDecoder(bytes.NewReader(metrics), format)

	ts := make([]prompb.TimeSeries, 0)

	for {
		var mf dto.MetricFamily

		switch err := dec.Decode(&mf); err {
		case nil:
			// got metrics
			mName := mf.GetName()
			mType := mf.GetType()
			isProbeSuccess := mName == "probe_success" && mType == dto.MetricType_GAUGE

			for _, m := range mf.GetMetric() {
				if isProbeSuccess && m.GetGauge().GetValue() == 0 {
					// TODO(mem): need to collect
					// logs from blackbox-exporter.
					//
					// We could run the probe with
					// debug=true and have custom
					// code to extract the logs and
					// the metrics.
					ts = appendDtoToTimeseries(nil, t, s.checkName, s.probeName, s.endpoint, mName, mType, m)

					err := fmt.Errorf("probe=%s endpoint=%s target=%s err=%w", s.probeName, s.endpoint, target, errCheckFailed)
					return ts, logs, err
				}

				ts = appendDtoToTimeseries(ts, t, s.checkName, s.probeName, s.endpoint, mName, mType, m)
			}

		case io.EOF:
			return ts, logs, nil

		default:
			return nil, nil, fmt.Errorf("decoding results from blackbox-exporter: %w", err)
		}
	}
}

func makeTimeseries(t time.Time, value float64, labels ...*prompb.Label) prompb.TimeSeries {
	var ts prompb.TimeSeries

	ts.Labels = make([]*prompb.Label, len(labels))
	copy(ts.Labels, labels)

	ts.Samples = []prompb.Sample{
		{Timestamp: t.UnixNano() / 1e6, Value: value},
	}

	return ts
}

func appendDtoToTimeseries(ts []prompb.TimeSeries, t time.Time, checkName, probeName, endpoint, mName string, mType dto.MetricType, metric *dto.Metric) []prompb.TimeSeries {
	baseLabels := []*prompb.Label{
		&prompb.Label{Name: "__name__", Value: mName},
		&prompb.Label{Name: "check", Value: checkName},
		&prompb.Label{Name: "probe", Value: probeName},
		&prompb.Label{Name: "endpoint", Value: endpoint},
	}

	var metricLabels []*prompb.Label

	if ml := metric.GetLabel(); len(ml) > 0 {
		metricLabels = make([]*prompb.Label, len(ml))
		for i, l := range ml {
			metricLabels[i] = &prompb.Label{Name: *(l.Name), Value: *(l.Value)}
		}
	}

	labels := make([]*prompb.Label, 0, len(baseLabels)+len(metricLabels))
	labels = append(labels, baseLabels...)
	labels = append(labels, metricLabels...)

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
			if q := s.GetQuantile(); q != nil {
				sLabels := make([]*prompb.Label, len(labels))
				copy(sLabels, labels)

				sLabels[0] = &prompb.Label{Name: "__name__", Value: mName + "_sum"}
				ts = append(ts, makeTimeseries(t, s.GetSampleSum(), sLabels...))

				sLabels[0] = &prompb.Label{Name: "__name__", Value: mName + "_count"}
				ts = append(ts, makeTimeseries(t, float64(s.GetSampleCount()), sLabels...))

				sLabels = make([]*prompb.Label, len(labels)+1)
				copy(sLabels, labels)

				for _, v := range q {
					sLabels[len(sLabels)-1] = &prompb.Label{
						Name:  "quantile",
						Value: strconv.FormatFloat(v.GetQuantile(), 'G', -1, 64),
					}
					ts = append(ts, makeTimeseries(t, v.GetValue(), sLabels...))
				}
			}
		}

	case dto.MetricType_HISTOGRAM:
		if h := metric.GetHistogram(); h != nil {
			if b := h.GetBucket(); b != nil {
				hLabels := make([]*prompb.Label, len(labels))
				copy(hLabels, labels)

				hLabels[0] = &prompb.Label{Name: "__name__", Value: mName + "_sum"}
				ts = append(ts, makeTimeseries(t, h.GetSampleSum(), hLabels...))

				hLabels[0] = &prompb.Label{Name: "__name__", Value: mName + "_count"}
				ts = append(ts, makeTimeseries(t, float64(h.GetSampleCount()), hLabels...))

				hLabels = make([]*prompb.Label, len(labels)+1)
				copy(hLabels, labels)

				for _, v := range b {
					hLabels[len(hLabels)-1] = &prompb.Label{
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
