package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/prompb"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

type TimeSeries []prompb.TimeSeries

// "github.com/prometheus/common/expfmt"

func scrape(ctx context.Context, publishCh chan<- TimeSeries, probeName string, target string, logger *log.Logger) {
	logger.Printf("scraping first set")
	if true {
		ts, err := collectData(ctx, time.Now(), probeName, target)
		if err != nil {
			logger.Printf("Error collecting data from %s: %s", target, err)
		}

		if ts != nil {
			publishCh <- ts
		}
	}

	const T = 5 * 1000 // period, ms

	ticker := time.NewTicker(T * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():

		case t := <-ticker.C:
			ts, err := collectData(ctx, t, probeName, target)
			if err != nil {
				log.Printf("Error collecting data from %s: %s", target, err)
				continue
			}

			if ts != nil {
				publishCh <- ts
			}
		}
	}
}

func collectData(ctx context.Context, t time.Time, probeName string, target string) (TimeSeries, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, fmt.Errorf("creating new request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting data from %s: %w", target, err)
	}

	defer func() {
		// drain body
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	format := expfmt.ResponseFormat(resp.Header)

	dec := expfmt.NewDecoder(resp.Body, format)

	ts := make([]prompb.TimeSeries, 0)

DECODE_LOOP:
	for {
		var metrics dto.MetricFamily

		switch err := dec.Decode(&metrics); err {
		case io.EOF:
			break DECODE_LOOP

		case nil:
			// got metrics
			for _, m := range metrics.GetMetric() {
				ts = appendDtoToTimeseries(ts, t, probeName, metrics.GetName(), metrics.GetType(), m)
			}

		default:
			return nil, err
		}
	}

	return ts, nil
}

func makeTimeseries(probeName string, name string, t time.Time, value float64, labels []*dto.LabelPair, extraLabels ...*prompb.Label) prompb.TimeSeries {
	var ts prompb.TimeSeries

	ts.Labels = make([]*prompb.Label, 0, len(labels)+2+len(extraLabels))
	ts.Labels = append(ts.Labels, &prompb.Label{Name: "__name__", Value: name})
	ts.Labels = append(ts.Labels, &prompb.Label{Name: "probe", Value: probeName})
	ts.Labels = append(ts.Labels, extraLabels...)
	for _, l := range labels {
		ts.Labels = append(ts.Labels, &prompb.Label{Name: *(l.Name), Value: *(l.Value)})
	}

	ts.Samples = []prompb.Sample{
		{Timestamp: t.UnixNano() / 1e6, Value: value},
	}

	return ts
}

func appendDtoToTimeseries(ts []prompb.TimeSeries, t time.Time, probeName string, mName string, mType dto.MetricType, metric *dto.Metric) []prompb.TimeSeries {
	switch mType {
	case dto.MetricType_COUNTER:
		if v := metric.GetCounter(); v != nil && v.Value != nil {
			ts = append(ts, makeTimeseries(probeName, mName, t, *v.Value, metric.GetLabel()))
		}

	case dto.MetricType_GAUGE:
		if v := metric.GetGauge(); v != nil && v.Value != nil {
			ts = append(ts, makeTimeseries(probeName, mName, t, *v.Value, metric.GetLabel()))
		}

	case dto.MetricType_UNTYPED:
		if v := metric.GetUntyped(); v != nil && v.Value != nil {
			ts = append(ts, makeTimeseries(probeName, mName, t, *v.Value, metric.GetLabel()))
		}

	case dto.MetricType_SUMMARY:
		if s := metric.GetSummary(); s != nil {
			if q := s.GetQuantile(); q != nil {
				ts = append(ts, makeTimeseries(probeName, mName+"_sum", t, s.GetSampleSum(), metric.GetLabel()))
				ts = append(ts, makeTimeseries(probeName, mName+"_count", t, float64(s.GetSampleCount()), metric.GetLabel()))

				for _, v := range q {
					ql := &prompb.Label{
						Name:  "quantile",
						Value: strconv.FormatFloat(v.GetQuantile(), 'G', -1, 64),
					}
					ts = append(ts, makeTimeseries(probeName, mName, t, v.GetValue(), metric.GetLabel(), ql))
				}
			}
		}

	case dto.MetricType_HISTOGRAM:
		if h := metric.GetHistogram(); h != nil {
			if b := h.GetBucket(); b != nil {
				ts = append(ts, makeTimeseries(probeName, mName+"_sum", t, h.GetSampleSum(), metric.GetLabel()))
				ts = append(ts, makeTimeseries(probeName, mName+"_count", t, float64(h.GetSampleCount()), metric.GetLabel()))
				for _, v := range b {
					bl := &prompb.Label{
						Name:  "le",
						Value: strconv.FormatFloat(v.GetUpperBound(), 'G', -1, 64),
					}
					ts = append(ts, makeTimeseries(probeName, mName, t, float64(v.GetCumulativeCount()), metric.GetLabel(), bl))
				}
			}
		}
	}

	return ts
}
