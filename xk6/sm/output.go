package sm

import (
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"go.k6.io/k6/metrics"
	"go.k6.io/k6/output"
)

const (
	ExtensionName = "sm"
	RawURLTagName = "__raw_url__"
)

func init() {
	output.RegisterExtension(ExtensionName, New)
}

// Output is a k6 output plugin that writes metrics to an io.Writer in
// Prometheus text exposition format.
type Output struct {
	logger logrus.FieldLogger
	buffer output.SampleBuffer
	out    io.WriteCloser
}

// New creates a new instance of the output.
func New(p output.Params) (output.Output, error) {
	fn := p.ConfigArgument
	if len(fn) == 0 {
		return nil, errors.New("output filename required")
	}

	fh, err := p.FS.Create(fn)
	if err != nil {
		return nil, err
	}

	return &Output{logger: p.Logger, out: fh}, nil
}

// Description returns a human-readable description of the output that will be
// shown in `k6 run`. For extensions it probably should include the version as
// well.
func (o *Output) Description() string {
	return "Synthetic Monitoring output"
}

// Start is called before the Engine tries to use the output and should be
// used for any long initialization tasks, as well as for starting a
// goroutine to asynchronously flush metrics to the output.
func (o *Output) Start() error {
	return nil
}

// AddMetricSamples receives the latest metric samples from the Engine.
//
// This method is called synchronously, so do not do anything blocking here
// that might take a long time. Preferably, just use the SampleBuffer or
// something like it to buffer metrics until they are flushed.
func (o *Output) AddMetricSamples(samples []metrics.SampleContainer) {
	o.buffer.AddMetricSamples(samples)
}

// Stop flushes all remaining metrics and finalize the test run.
func (o *Output) Stop() error {
	defer o.out.Close()

	genericMetrics := newGenericMetricsCollection()
	targetMetrics := newTargetMetricsCollection()

	for _, samples := range o.buffer.GetBufferedSamples() {
		for _, sample := range samples.GetSamples() {
			tags := getTags(sample)

			scenario := tags["scenario"]
			group := tags["group"]

			if _, found := tags["name"]; found {
				targetMetrics.Update(sample, scenario, group, tags)
				continue
			}

			// The samples that don't have "name" in their tags seem to be generic metrics about various
			// things.
			//
			// Seen so far:
			//
			// * checks -- this seems to be the number of checks performed (the number of times the check
			//   function is called?) and the "check" tag might be different each time? But it seems to emit
			//   one instance of "check" each time the function is called, even if it's with the same
			//   "check" tag.
			// * data_received -- this is the total for all requests in the scenario
			// * data_sent -- this is the total for all requests in the scenario
			// * iteration_duration -- the duration for the iteration in ms; with scenarios there seems to
			//   be one iteration per scenario.
			// * iterations -- not interesting, how many iterations in each scenario

			metricName, value := deriveMetricNameAndValue(sample)
			switch metricName {
			case "":
				continue

			case "checks_total":
				// "checks" is a little weird. It seems to be the number of checks performed
				// (the number of times the check function is called?) and the "check" tag might
				// be different each time becuase it's the name of the check provided as an
				// argument to the check function. One of these samples seems to be emitted each
				// time the check function is called.
				//
				// The tag describing the check is called "check", so we end up with a metric
				// "check" with a label "check".
				//
				// The problem with this is that the check name seems to be free-form, so we
				// might end up with invalid label values. This is probably a job for Loki,
				// meaning we need an structured way of storing this information in logs.
				fields := logrus.Fields{
					"source":   ExtensionName,
					"metric":   metricName,
					"scenario": scenario,
					"value":    value,
				}
				if group != "" {
					fields["group"] = group
				}
				for k, v := range tags {
					fields[k] = v
				}
				entry := o.logger.WithFields(fields)
				entry.Info("check result")

				delete(tags, "check")

				// Now we need to do something weird: because the _value_ of the check metric is 0 if
				// the check fails. If that's the case, add a tag result="fail" and set the value to 1
				// (so that the metric is counting failures), otherwise add result="pass".
				if value == 0 {
					tags["result"] = "fail"
					value = 1
				} else {
					tags["result"] = "pass"
				}
			}

			genericMetrics.Update(metricName, scenario, group, value, tags)
		}
	}

	// It might be a good idea to remove the tags from each of the metrics and instead create an scenario_info
	// metric. The problem with this is that 1) there might be no scenario (in which case it might be named
	// default?); 2) it's technically possible to add tags to invidual requests via request options.

	genericMetrics.Write(o.out)

	targetMetrics.Write(o.out)

	return nil
}

func getTags(sample metrics.Sample) map[string]string {
	var tags map[string]string
	if sample.Tags != nil {
		tags = sample.Tags.Map()
	}

	// The documentation at https://k6.io/docs/using-k6/tags-and-groups/ seems to suggest that
	// "group" should not be empty (it shouldn't be there if there's a single group), but I keep
	// seeing instances of an empty group name.
	if group, found := tags["group"]; found && group == "" {
		delete(tags, "group")
	}

	return tags
}

func deriveMetricNameAndValue(sample metrics.Sample) (string, float64) {
	metricName := sample.TimeSeries.Metric.Name
	value := sample.Value

	switch metricName {
	case "iterations":
		metricName = ""
		value = 0

	case "checks":
		metricName = "checks_total"

	case "iteration_duration":
		metricName = "iteration_duration_seconds"
		value /= 1000

	case "data_sent":
		metricName = "data_sent_bytes"

	case "data_received":
		metricName = "data_received_bytes"
	}

	return metricName, value
}

type targetId struct {
	url      string
	method   string
	scenario string
	group    string
	name     string
}

type targetMetrics struct {
	requests         int
	failed           int
	expectedResponse bool
	group            string
	scenario         string

	// HTTP info
	proto      string
	tlsVersion string
	status     []string

	// timings
	duration       []float64
	blocked        []float64
	connecting     []float64
	sending        []float64
	waiting        []float64
	receiving      []float64
	tlsHandshaking []float64

	tags map[string]string
}

type targetMetricsCollection map[targetId]targetMetrics

func newTargetMetricsCollection() targetMetricsCollection {
	return make(targetMetricsCollection)
}

func (collection targetMetricsCollection) Update(sample metrics.Sample, scenario, group string, tags map[string]string) {
	key := targetId{
		url:      getURL(tags),
		method:   tags["method"],
		scenario: scenario,
		group:    group,
		name:     tags["name"],
	}

	// the metrics for this target
	tm := collection[key]

	tm.scenario = scenario
	tm.group = group

	switch sample.TimeSeries.Metric.Name {
	case "http_reqs":
		tm.requests += int(sample.Value)
		tm.proto = tags["proto"]
		tm.tlsVersion = tags["tls_version"]
		tm.status = append(tm.status, tags["status"])
	case "http_req_duration":
		tm.duration = append(tm.duration, sample.Value/1000) // ms
	case "http_req_blocked":
		tm.blocked = append(tm.blocked, sample.Value/1000) // ms
	case "http_req_connecting":
		tm.connecting = append(tm.connecting, sample.Value/1000) // ms
	case "http_req_tls_handshaking":
		tm.tlsHandshaking = append(tm.tlsHandshaking, sample.Value/1000) // ms
	case "http_req_sending":
		tm.sending = append(tm.sending, sample.Value/1000) // ms
	case "http_req_waiting":
		tm.waiting = append(tm.waiting, sample.Value/1000) // ms
	case "http_req_receiving":
		tm.receiving = append(tm.receiving, sample.Value/1000) // ms
	case "http_req_failed":
		tm.failed += int(sample.Value)
	}

	// Remove elements from tags because the following are stored in dedicated fields.

	delete(tags, "url")
	delete(tags, RawURLTagName)
	delete(tags, "method")
	delete(tags, "scenario")
	delete(tags, "group")
	delete(tags, "name")

	delete(tags, "proto")
	delete(tags, "tls_version")
	delete(tags, "status")

	tm.tags = tags

	collection[key] = tm
}

func (c targetMetricsCollection) Write(w io.Writer) {
	for key, ti := range c {
		out := newBufferedMetricTextOutput(w, "url", key.url, "method", key.method)
		if key.scenario != "" {
			out.Tags("scenario", key.scenario)
		}
		if key.group != "" {
			out.Tags("group", key.group)
		}
		if key.name != "" {
			out.Tags("name", key.name)
		}

		// Remove expected_reponse from tags and write it as a separate
		// metric. It reads weirdly as a label, specially one that is
		// applied to all the metrics.
		expectedResponse := ti.tags["expected_response"]
		delete(ti.tags, "expected_response")

		out.Name("probe_http_got_expected_response")
		if expectedResponse == "false" {
			out.Value(0)
		} else {
			out.Value(1)
		}

		// Remove error code from tags and write it as a separate
		// metric because the possible values span ~ 700 values.
		errorCode := ti.tags["error_code"]
		delete(ti.tags, "error_code")

		out.Name("probe_http_error_code")
		if errorCode == "" || errorCode == "0" {
			out.Value(0)
		} else if v, err := strconv.Atoi(errorCode); err != nil {
			out.Value(-1)
		} else {
			out.Value(v)
		}

		out.Name("probe_http_info")
		if ti.tlsVersion != "" {
			out.KeyValue(`tls_version`, strings.TrimPrefix(ti.tlsVersion, "tls"))
		}
		// If the request failed, proto might be empty because there
		// was no response.
		if len(ti.proto) > 0 {
			out.KeyValue("proto", ti.proto)
		}

		for k, v := range ti.tags {
			out.KeyValue(k, v)
		}
		out.Value(1)

		out.Name("probe_http_requests_total")
		out.Value(ti.requests)

		out.Name("probe_http_requests_failed_total")
		out.Value(ti.failed)

		// TODO(mem): decide what to do with failed requests.
		//
		// If a request fails, depending on the reason, some of the
		// timings might be missing. This means that we might skew the
		// results towards 0 if we try to do over-time aggregations.

		out.Name(`probe_http_status_code`)
		out.Value(ti.status[0])

		if protoVersion := strings.TrimPrefix(strings.ToLower(ti.proto), "http/"); len(protoVersion) > 0 {
			out.Name(`probe_http_version`)
			out.Value(protoVersion) // XXX
		}

		out.Name(`probe_http_ssl`)
		if ti.tlsVersion == "" {
			out.Value(0)
		} else {
			out.Value(1)
		}

		if ti.requests == 1 {
			out.Name("probe_http_duration_seconds")
			out.KeyValue("phase", "resolve")
			out.Value(0)

			out.Name("probe_http_duration_seconds")
			out.KeyValue("phase", "connect")
			out.Value(ti.connecting[0])

			out.Name("probe_http_duration_seconds")
			out.KeyValue("phase", "tls")
			out.Value(ti.tlsHandshaking[0])

			out.Name("probe_http_duration_seconds")
			out.KeyValue("phase", "processing")
			out.Value(ti.waiting[0])

			out.Name("probe_http_duration_seconds")
			out.KeyValue("phase", "transfer")
			out.Value(ti.receiving[0])

			out.Name("probe_http_total_duration_seconds")
			out.Value(ti.duration[0])

			// ti.sending: writing the request
			// ti.blocked: waiting for the connection to be available
		} else {
			out.Name("probe_http_duration_seconds")
			out.KeyValue("phase", "resolve")
			out.Stats(make([]float64, ti.requests))

			out.Name("probe_http_duration_seconds")
			out.KeyValue("phase", "connect")
			out.Stats(ti.connecting)

			out.Name("probe_http_duration_seconds")
			out.KeyValue("phase", "tls")
			out.Stats(ti.tlsHandshaking)

			out.Name("probe_http_duration_seconds")
			out.KeyValue("phase", "processing")
			out.Stats(ti.waiting)

			out.Name("probe_http_duration_seconds")
			out.KeyValue("phase", "transfer")
			out.Stats(ti.receiving)

			out.Name("probe_http_total_duration_seconds")
			out.Stats(ti.duration)
		}
	}
}

type genericMetric struct {
	name  string
	value float64
	tags  map[string]string
}

type genericMetricsCollection map[string]genericMetric

func newGenericMetricsCollection() genericMetricsCollection {
	return make(genericMetricsCollection)
}

func (c genericMetricsCollection) Update(metric, scenario, group string, delta float64, tags map[string]string) {
	var key strings.Builder

	key.WriteString(metric)
	key.WriteString(scenario)
	key.WriteString(group)

	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		key.WriteString(k)
		key.WriteString(tags[k])
	}

	keyStr := key.String()

	m := c[keyStr]
	m.name = metric
	m.value += delta
	if len(tags) > 0 {
		m.tags = tags
	}
	c[keyStr] = m
}

func (c genericMetricsCollection) Write(w io.Writer) {
	for _, metric := range c {
		out := newBufferedMetricTextOutput(w)
		out.Name("probe_" + metric.name)
		for key, value := range metric.tags {
			out.Tags(key, value)
		}
		// output stats instead?
		out.Value(metric.value)
	}
}

type immediateMetricTextOutput struct {
	dest                io.Writer
	commonKeysAndValues []string
	count               int
}

func newMetricTextOutput(dest io.Writer, keysAndValues ...string) *immediateMetricTextOutput {
	return &immediateMetricTextOutput{dest: dest, commonKeysAndValues: keysAndValues}
}

func (o *immediateMetricTextOutput) Name(name string) {
	fmt.Fprint(o.dest, name)
	fmt.Fprint(o.dest, "{")
	o.count = 0
}

func (o *immediateMetricTextOutput) KeyValue(key, value string) {
	if o.count > 0 {
		fmt.Fprint(o.dest, ",")
	}

	if !isValidMetricName(key) {
		key = sanitizeLabelName(key)
	}

	fmt.Fprint(o.dest, key)
	fmt.Fprint(o.dest, `="`)
	fmt.Fprint(o.dest, value)
	fmt.Fprint(o.dest, `"`)

	o.count++
}

func (o *immediateMetricTextOutput) Value(v any) {
	for i := 0; i < len(o.commonKeysAndValues); i += 2 {
		key := o.commonKeysAndValues[i]
		if !isValidMetricName(key) {
			key = sanitizeLabelName(key)
		}
		o.KeyValue(key, o.commonKeysAndValues[i+1])
	}
	fmt.Fprint(o.dest, "} ")
	fmt.Fprintln(o.dest, v)
}

type bufferedMetricTextOutput struct {
	dest                io.Writer
	commonKeysAndValues []string
	name                string
	buf                 strings.Builder
}

func newBufferedMetricTextOutput(dest io.Writer, keysAndValues ...string) *bufferedMetricTextOutput {
	return &bufferedMetricTextOutput{dest: dest, commonKeysAndValues: keysAndValues}
}

func (o *bufferedMetricTextOutput) Name(name string) {
	o.name = name
	o.buf.Reset()
}

func (o *bufferedMetricTextOutput) Tags(keysAndValues ...string) {
	o.commonKeysAndValues = append(o.commonKeysAndValues, keysAndValues...)
}

func (o *bufferedMetricTextOutput) KeyValue(key, value string) {
	if o.buf.Len() > 0 {
		o.buf.WriteRune(',')
	}

	if !isValidMetricName(key) {
		key = sanitizeLabelName(key)
	}

	o.buf.WriteString(key)
	o.buf.WriteRune('=')
	o.buf.WriteRune('"')
	o.buf.WriteString(value)
	o.buf.WriteRune('"')
}

func (o *bufferedMetricTextOutput) Value(v any) {
	for i := 0; i < len(o.commonKeysAndValues); i += 2 {
		if o.buf.Len() > 0 {
			o.buf.WriteRune(',')
		}

		key := o.commonKeysAndValues[i]
		if !isValidMetricName(key) {
			key = sanitizeLabelName(key)
		}

		o.buf.WriteString(key)
		o.buf.WriteRune('=')
		o.buf.WriteRune('"')
		o.buf.WriteString(o.commonKeysAndValues[i+1])
		o.buf.WriteRune('"')
	}

	fmt.Fprint(o.dest, o.name)
	fmt.Fprint(o.dest, "{")
	fmt.Fprint(o.dest, o.buf.String())
	fmt.Fprint(o.dest, "} ")
	fmt.Fprintln(o.dest, v)
}

func (o *bufferedMetricTextOutput) Stats(v []float64) {
	for i := 0; i < len(o.commonKeysAndValues); i += 2 {
		if o.buf.Len() > 0 {
			o.buf.WriteRune(',')
		}

		key := o.commonKeysAndValues[i]
		if !isValidMetricName(key) {
			key = sanitizeLabelName(key)
		}

		o.buf.WriteString(key)
		o.buf.WriteRune('=')
		o.buf.WriteRune('"')
		o.buf.WriteString(o.commonKeysAndValues[i+1])
		o.buf.WriteRune('"')
	}

	stats := getStats(v)

	fmt.Fprint(o.dest, o.name)
	fmt.Fprint(o.dest, "_min")
	fmt.Fprint(o.dest, "{")
	fmt.Fprint(o.dest, o.buf.String())
	fmt.Fprint(o.dest, "} ")
	fmt.Fprintln(o.dest, stats.min)

	fmt.Fprint(o.dest, o.name)
	fmt.Fprint(o.dest, "_max")
	fmt.Fprint(o.dest, "{")
	fmt.Fprint(o.dest, o.buf.String())
	fmt.Fprint(o.dest, "} ")
	fmt.Fprintln(o.dest, stats.max)

	fmt.Fprint(o.dest, o.name)
	// fmt.Fprint(o.dest, "_mean")
	fmt.Fprint(o.dest, "{")
	fmt.Fprint(o.dest, o.buf.String())
	fmt.Fprint(o.dest, "} ")
	fmt.Fprintln(o.dest, stats.med)

	fmt.Fprint(o.dest, o.name)
	fmt.Fprint(o.dest, "_count")
	fmt.Fprint(o.dest, "{")
	fmt.Fprint(o.dest, o.buf.String())
	fmt.Fprint(o.dest, "} ")
	fmt.Fprintln(o.dest, stats.n)

	fmt.Fprint(o.dest, o.name)
	fmt.Fprint(o.dest, "_sum")
	fmt.Fprint(o.dest, "{")
	fmt.Fprint(o.dest, o.buf.String())
	fmt.Fprint(o.dest, "} ")
	fmt.Fprintln(o.dest, stats.sum)
}

type stats struct {
	n   int
	min float64
	max float64
	med float64
	sum float64
}

func getStats(a []float64) stats {
	sort.Float64s(a)

	out := stats{
		n:   len(a),
		min: a[0],
		max: a[len(a)-1],
	}

	for _, v := range a {
		out.sum += v
	}

	if out.n > 1 {
		p, f := modf(float64(out.n-1) * 0.5)
		out.med = lerp(a[p], a[p+1], f)
	} else {
		out.med = out.min
	}

	return out
}

// lerp returns the linear interpolation between a and b at t.
func lerp(a, b, t float64) float64 {
	return (1-t)*a + t*b
}

// modf returns the integer and fractional parts of n.
func modf(n float64) (int, float64) {
	i, f := math.Modf(n)
	return int(i), f
}

var validMetricNameRe = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)

// isValidMetricNameRe returns true iff s is a valid metric name.
func isValidMetricNameRe(s string) bool {
	return validMetricNameRe.MatchString(s)
}

// isValidMetricName returns true iff s is a valid metric name.
//
// This function is a faster hardcoded implementation wrt to the regular expression.
func isValidMetricName(s string) bool {
	if len(s) == 0 {
		return false
	}

	for i, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || r == ':' || (r >= '0' && r <= '9' && i > 0)) {
			return false
		}
	}

	return true
}

// sanitizeLabelName replaces all invalid characters in s with '_'.
func sanitizeLabelName(s string) string {
	var builder strings.Builder

	for i, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || r == ':' || (r >= '0' && r <= '9' && i > 0) {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('_')
		}
	}

	return builder.String()
}

func getURL(m map[string]string) string {
	if u := m[RawURLTagName]; u != "" {
		return u
	}

	return m["url"]
}
