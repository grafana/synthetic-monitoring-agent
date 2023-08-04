package pusher

import (
	"time"

	"github.com/prometheus/prometheus/prompb"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/logproto"
)

const (
	// MetricsMaxGap is the maximum interval between two consecutive samples of a timeseries so that
	// there is no discontinuity in it.
	MetricsMaxGap = 5 * time.Minute

	// maxHoles is the maximum number of gaps that is accepted between two consecutive samples of a timeseries
	// for the purposes of making this metric continuous.
	// If a gap in a timeseries is found that requires adding more than this number of extra samples, then it's
	// ignored and the discontinuity left as is.
	maxHoles = 5
)

// MetricGapFiller wraps a Publisher and pads the timeseries with repeated samples so that there is no
// discontinuity in them. This is useful to prevent discontinuities in metrics that have a generation period
// longer that MetricsMaxGap.
type MetricGapFiller struct {
	MaxGap      time.Duration
	Publisher   Publisher
	KnownSeries map[string]prompb.Sample
}

// Publish passes the Payload to the wrapped publisher, padding the metrics if necessary.
func (m *MetricGapFiller) Publish(p Payload) {
	m.Publisher.Publish(payloadImpl{
		tenant: p.Tenant(),
		series: m.process(p.Metrics()),
		logs:   p.Streams(),
	})
}

func (m *MetricGapFiller) process(next []prompb.TimeSeries) []prompb.TimeSeries {
	series := make(map[string]prompb.Sample, len(next))
	output := make([]prompb.TimeSeries, len(next))

	for i := range next {
		output[i] = next[i]
		if !isSupported(next[i]) {
			continue
		}
		key, err := seriesKey(next[i])
		if err != nil {
			continue
		}
		series[key] = next[i].Samples[0]
		if prev, seen := m.KnownSeries[key]; seen {
			output[i].Samples = fillGaps(prev, next[i].Samples, m.MaxGap)
		}
	}

	m.KnownSeries = series
	return output
}

func fillGaps(old prompb.Sample, newSlice []prompb.Sample, maxGap time.Duration) []prompb.Sample {
	// Here newSlice has already been validated to have length 1, but just in case.
	if len(newSlice) != 1 {
		return newSlice
	}
	new := newSlice[0]
	oldTime := time.UnixMilli(old.Timestamp)
	newTime := time.UnixMilli(new.Timestamp)
	diff := newTime.Sub(oldTime)
	if diff <= maxGap {
		return newSlice
	}
	repeats := int64(diff / maxGap)
	if repeats > maxHoles {
		return newSlice
	}
	if diff%maxGap == 0 {
		repeats--
	}
	result := make([]prompb.Sample, repeats+1)
	for i, base := int64(0), oldTime.Add(maxGap); i < repeats; i, base = i+1, base.Add(maxGap) {
		result[i].Timestamp = base.UnixMilli()
		result[i].Value = old.Value
	}
	result[repeats] = new
	return result
}

func seriesKey(s prompb.TimeSeries) (string, error) {
	obj := prompb.Labels{
		Labels: s.Labels,
	}
	data, err := obj.Marshal()
	return string(data), err
}

func isSupported(s prompb.TimeSeries) bool {
	return len(s.Samples) == 1 && len(s.Exemplars) == 0 && len(s.Histograms) == 0
}

type payloadImpl struct {
	tenant model.GlobalID
	series []prompb.TimeSeries
	logs   []logproto.Stream
}

func (p payloadImpl) Tenant() model.GlobalID {
	return p.tenant
}

func (p payloadImpl) Metrics() []prompb.TimeSeries {
	return p.series
}

func (p payloadImpl) Streams() []logproto.Stream {
	return p.logs
}
