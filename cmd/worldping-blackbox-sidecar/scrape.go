package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/prompb"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

type TimeSeries []prompb.TimeSeries

// "github.com/prometheus/common/expfmt"

type scraper struct {
	publishCh chan<- TimeSeries
	probeName string
	target    string
	endpoint  string
	logger    logger
}

var errProbeFailed = errors.New("probe failed")

func (s scraper) run(ctx context.Context) {
	s.logger.Printf("starting scraper at %s for %s", s.probeName, s.target)

	scrape := func(ctx context.Context, t time.Time) {
		switch ts, err := s.collectData(ctx, t); {
		case errors.Is(err, errProbeFailed):

		case err != nil:
			s.logger.Printf("Error collecting data from %s: %s", s.target, err)

		default:
			if ts != nil {
				s.publishCh <- ts
			}
		}
	}

	if true {
		s.logger.Printf("scraping first set")
		scrape(ctx, time.Now())
	}

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

func (s scraper) collectData(ctx context.Context, t time.Time) (TimeSeries, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.target, nil)
	if err != nil {
		return nil, fmt.Errorf("creating new request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting data from %s: %w", s.target, err)
	}

	defer func() {
		// drain body
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	format := expfmt.ResponseFormat(resp.Header)

	dec := expfmt.NewDecoder(resp.Body, format)

	ts := make([]prompb.TimeSeries, 0)

	for {
		var metrics dto.MetricFamily

		switch err := dec.Decode(&metrics); err {
		case nil:
			// got metrics
			mName := metrics.GetName()
			mType := metrics.GetType()
			isProbeSuccess := mName == "probe_success" && mType == dto.MetricType_GAUGE

			for _, m := range metrics.GetMetric() {
				if isProbeSuccess && m.GetGauge().GetValue() == 0 {
					// TODO(mem): need to collect
					// logs from blackbox-exporter.
					//
					// We could run the probe with
					// debug=true and have custom
					// code to extract the logs and
					// the metrics.
					s.logger.Printf("probe=%s endpoint=%s target=%s err=%s", s.probeName, s.endpoint, s.target, errProbeFailed)
					ts = appendDtoToTimeseries(nil, t, s.probeName, s.endpoint, mName, mType, m)

					return ts, nil
				}

				ts = appendDtoToTimeseries(ts, t, s.probeName, s.endpoint, mName, mType, m)
			}

		case io.EOF:
			return ts, nil

		default:
			return nil, err
		}
	}

}

func makeTimeseries(probeName string, name string, endpoint string, t time.Time, value float64, labels []*dto.LabelPair, extraLabels ...*prompb.Label) prompb.TimeSeries {
	var ts prompb.TimeSeries

	ts.Labels = make([]*prompb.Label, 0, len(labels)+2+len(extraLabels))
	ts.Labels = append(ts.Labels, &prompb.Label{Name: "__name__", Value: name})
	ts.Labels = append(ts.Labels, &prompb.Label{Name: "probe", Value: probeName})
	ts.Labels = append(ts.Labels, &prompb.Label{Name: "endpoint", Value: endpoint})
	ts.Labels = append(ts.Labels, extraLabels...)
	for _, l := range labels {
		ts.Labels = append(ts.Labels, &prompb.Label{Name: *(l.Name), Value: *(l.Value)})
	}

	ts.Samples = []prompb.Sample{
		{Timestamp: t.UnixNano() / 1e6, Value: value},
	}

	return ts
}

func appendDtoToTimeseries(ts []prompb.TimeSeries, t time.Time, probeName string, endpoint string, mName string, mType dto.MetricType, metric *dto.Metric) []prompb.TimeSeries {
	switch mType {
	case dto.MetricType_COUNTER:
		if v := metric.GetCounter(); v != nil && v.Value != nil {
			ts = append(ts, makeTimeseries(probeName, mName, endpoint, t, *v.Value, metric.GetLabel()))
		}

	case dto.MetricType_GAUGE:
		if v := metric.GetGauge(); v != nil && v.Value != nil {
			ts = append(ts, makeTimeseries(probeName, mName, endpoint, t, *v.Value, metric.GetLabel()))
		}

	case dto.MetricType_UNTYPED:
		if v := metric.GetUntyped(); v != nil && v.Value != nil {
			ts = append(ts, makeTimeseries(probeName, mName, endpoint, t, *v.Value, metric.GetLabel()))
		}

	case dto.MetricType_SUMMARY:
		if s := metric.GetSummary(); s != nil {
			if q := s.GetQuantile(); q != nil {
				ts = append(ts, makeTimeseries(probeName, mName+"_sum", endpoint, t, s.GetSampleSum(), metric.GetLabel()))
				ts = append(ts, makeTimeseries(probeName, mName+"_count", endpoint, t, float64(s.GetSampleCount()), metric.GetLabel()))

				for _, v := range q {
					ql := &prompb.Label{
						Name:  "quantile",
						Value: strconv.FormatFloat(v.GetQuantile(), 'G', -1, 64),
					}
					ts = append(ts, makeTimeseries(probeName, mName, endpoint, t, v.GetValue(), metric.GetLabel(), ql))
				}
			}
		}

	case dto.MetricType_HISTOGRAM:
		if h := metric.GetHistogram(); h != nil {
			if b := h.GetBucket(); b != nil {
				ts = append(ts, makeTimeseries(probeName, mName+"_sum", endpoint, t, h.GetSampleSum(), metric.GetLabel()))
				ts = append(ts, makeTimeseries(probeName, mName+"_count", endpoint, t, float64(h.GetSampleCount()), metric.GetLabel()))
				for _, v := range b {
					bl := &prompb.Label{
						Name:  "le",
						Value: strconv.FormatFloat(v.GetUpperBound(), 'G', -1, 64),
					}
					ts = append(ts, makeTimeseries(probeName, mName, endpoint, t, float64(v.GetCumulativeCount()), metric.GetLabel(), bl))
				}
			}
		}
	}

	return ts
}
