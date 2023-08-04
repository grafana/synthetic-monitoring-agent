package pusher

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/logproto"
)

func TestGap(t *testing.T) {
	withHistogram := func(s prompb.TimeSeries, h prompb.Histogram) prompb.TimeSeries {
		s.Histograms = append(s.Histograms, h)
		return s
	}

	withExemplars := func(s prompb.TimeSeries, e prompb.Exemplar) prompb.TimeSeries {
		s.Exemplars = append(s.Exemplars, e)
		return s
	}

	baseTime := time.Now()
	makeLogs := func(delta time.Duration) []logproto.Stream {
		return []logproto.Stream{
			{
				Labels: "a=b,c=d",
				Entries: []logproto.Entry{
					{Timestamp: baseTime.Add(delta), Line: "hello world"},
					{Timestamp: baseTime.Add(delta), Line: "this is a log message"},
				},
			},
		}
	}

	// Format
	// ======
	//
	// The format for representing a timeseries here is:
	//
	// metric_name{"key"="value",...} value@time[,value2@time2...]
	//
	// This results in a prompb.TimeSeries with Labels __name__="metric_name", the rest of the labels,
	// and one or more samples with the given value and timestamp.
	//
	// Expected
	// ========
	//
	// A missing expected field for a test means the output is expected to be the same
	// as the input.g
	type subtest struct {
		input, expected Payload
	}
	for title, test := range map[string]struct {
		filler MetricGapFiller
		tests  []subtest
	}{
		"no gap": {
			filler: MetricGapFiller{MaxGap: 5 * time.Millisecond},
			tests: []subtest{
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@0`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@0`),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@1`),
							stringToSeries(`other_metric{"foo"="bar"} = 1@1`),
						},
					},
				},
			},
		},

		"single gap": {
			filler: MetricGapFiller{MaxGap: 5 * time.Millisecond},
			tests: []subtest{
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@0`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@0`),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@10`),
							stringToSeries(`other_metric{"foo"="bar"} = 1@10`),
						},
					},
					expected: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@5,2@10`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@5,1@10`),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 3@20`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@20`),
						},
					},
					expected: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@15,3@20`),
							stringToSeries(`other_metric{"foo"="bar"} = 1@15,0@20`),
						},
					},
				},
			},
		},

		"large gap": {
			filler: MetricGapFiller{MaxGap: 5 * time.Millisecond},
			tests: []subtest{
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@0`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@0`),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@20`),
							stringToSeries(`other_metric{"foo"="bar"} = 1@10`),
						},
					},
					expected: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@5,1.5@10,1.5@15,2@20`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@5,1@10`),
						},
					},
				},
			},
		},

		"legitimate gap in metrics": {
			filler: MetricGapFiller{MaxGap: 5 * time.Millisecond},
			tests: []subtest{
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 10@0`),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 20@5`),
							stringToSeries(`other_metric{"foo"="bar"} = 1@5`),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`other_metric{"foo"="bar"} = 2@10`),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 30@15`),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`other_metric{"foo"="bar"} = 0@20`),
						},
					},
				},
			},
		},

		"gap too big": {
			filler: MetricGapFiller{MaxGap: 5 * time.Millisecond},
			tests: []subtest{
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@0`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@0`),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@30`),
							stringToSeries(`other_metric{"foo"="bar"} = 1@30`),
						},
					},
				},
			},
		},

		"ignore metrics with more than one sample": {
			filler: MetricGapFiller{MaxGap: 5 * time.Millisecond},
			tests: []subtest{
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@0,2@5`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@0`),
							stringToSeries(`third_metric{"foo"="bar"} = 10@0`),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@20`),
							stringToSeries(`other_metric{"foo"="bar"} = 1@10,2@15`),
							stringToSeries(`third_metric{"foo"="bar"} = 10@15`),
						},
					},
					expected: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@20`),
							stringToSeries(`other_metric{"foo"="bar"} = 1@10,2@15`),
							stringToSeries(`third_metric{"foo"="bar"} = 10@5,10@10,10@15`),
						},
					},
				},
			},
		},

		"ignore timeseries with histograms": {
			filler: MetricGapFiller{MaxGap: 5 * time.Millisecond},
			tests: []subtest{
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@0`),
							withHistogram(
								stringToSeries(`histogram_metric{"foo"="bar"} = 0@0`),
								prompb.Histogram{Sum: 5},
							),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@10`),
							withHistogram(
								stringToSeries(`histogram_metric{"foo"="bar"} = 1@10`),
								prompb.Histogram{Sum: 10, Timestamp: 10},
							),
						},
					},
					expected: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@5,2@10`),
							withHistogram(
								stringToSeries(`histogram_metric{"foo"="bar"} = 1@10`),
								prompb.Histogram{Sum: 10, Timestamp: 10},
							),
						},
					},
				},
			},
		},

		"ignore timeseries with exemplars": {
			filler: MetricGapFiller{MaxGap: 5 * time.Millisecond},
			tests: []subtest{
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@0`),
							withExemplars(
								stringToSeries(`exemplars_metric{"foo"="bar"} = 0@0`),
								prompb.Exemplar{Value: 1},
							),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@10`),
							withExemplars(
								stringToSeries(`exemplars_metric{"foo"="bar"} = 100@10`),
								prompb.Exemplar{Value: 100, Timestamp: 10},
							),
						},
					},
					expected: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@5,2@10`),
							withExemplars(
								stringToSeries(`exemplars_metric{"foo"="bar"} = 100@10`),
								prompb.Exemplar{Value: 100, Timestamp: 10},
							),
						},
					},
				},
			},
		},

		"pass logs": {
			filler: MetricGapFiller{MaxGap: 5 * time.Millisecond},
			tests: []subtest{
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@0`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@0`),
						},
						logs: makeLogs(0),
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@10`),
							stringToSeries(`other_metric{"foo"="bar"} = 1@10`),
						},
						logs: makeLogs(100),
					},
					expected: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@5,2@10`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@5,1@10`),
						},
						logs: makeLogs(100),
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 3@20`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@20`),
						},
					},
					expected: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@15,3@20`),
							stringToSeries(`other_metric{"foo"="bar"} = 1@15,0@20`),
						},
					},
				},
			},
		},

		"small variance": {
			filler: MetricGapFiller{MaxGap: 5 * time.Millisecond},
			tests: []subtest{
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@0`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@0`),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@9`),
							stringToSeries(`other_metric{"foo"="bar"} = 1@9`),
						},
					},
					expected: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 1.5@5,2@9`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@5,1@9`),
						},
					},
				},
				{
					input: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 3@18`),
							stringToSeries(`other_metric{"foo"="bar"} = 0@18`),
						},
					},
					expected: payloadImpl{
						tenant: 1,
						series: []prompb.TimeSeries{
							stringToSeries(`my_metric{"foo"="bar"} = 2@14,3@18`),
							stringToSeries(`other_metric{"foo"="bar"} = 1@14,0@18`),
						},
					},
				},
			},
		},
	} {
		t.Run(title, func(t *testing.T) {
			for idx, subcase := range test.tests {
				var recorder recordingPublisher
				test.filler.Publisher = &recorder
				test.filler.Publish(subcase.input)
				if subcase.expected == nil {
					subcase.expected = subcase.input
				}
				require.Equal(t, subcase.expected, recorder.last, idx)
			}
		})
	}
}

type recordingPublisher struct {
	last Payload
}

func (r *recordingPublisher) Publish(p Payload) {
	r.last = p
}

var decodeRegexp = regexp.MustCompile(`^([^\ {]*){(.*)} = ([0-9\.@,]+)$`)

func stringToSeries(str string) prompb.TimeSeries {
	m := decodeRegexp.FindStringSubmatch(str)
	if len(m) != 4 {
		panic(str)
	}

	var l []prompb.Label
	if len(m[1]) > 0 {
		l = append(l, prompb.Label{
			Name:  "__name__",
			Value: m[1],
		})
	}
	var err error
	for _, chunk := range strings.Split(m[2], ",") {
		parts := strings.Split(chunk, "=")
		if len(parts) != 2 {
			panic(parts)
		}
		var lab prompb.Label
		lab.Name, err = strconv.Unquote(parts[0])
		if err != nil {
			panic(err)
		}
		lab.Value, err = strconv.Unquote(parts[1])
		if err != nil {
			panic(err)
		}
		l = append(l, lab)
	}

	var s []prompb.Sample
	for _, chunk := range strings.Split(m[3], ",") {
		var err error
		parts := strings.Split(chunk, "@")
		if len(parts) != 2 {
			panic(parts)
		}
		var sample prompb.Sample
		sample.Value, err = strconv.ParseFloat(parts[0], 64)
		if err != nil {
			panic(err)
		}
		sample.Timestamp, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			panic(err)
		}
		s = append(s, sample)
	}

	return prompb.TimeSeries{
		Labels:  l,
		Samples: s,
	}
}
