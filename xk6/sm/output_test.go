package sm

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"go.k6.io/k6/metrics"
	"go.k6.io/k6/output"
)

func TestOutputNew(t *testing.T) {
	testcases := map[string]struct {
		input       output.Params
		expectError bool
	}{
		"happy path": {
			input:       output.Params{ConfigArgument: "test.out", FS: afero.NewMemMapFs()},
			expectError: false,
		},
		"no filename": {
			input:       output.Params{ConfigArgument: "", FS: afero.NewMemMapFs()},
			expectError: true,
		},
		"cannot create file": {
			input:       output.Params{ConfigArgument: "test.out", FS: afero.NewReadOnlyFs(afero.NewMemMapFs())},
			expectError: true,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := New(tc.input)
			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, actual)
			} else {
				require.NoError(t, err)
				require.NotNil(t, actual)
			}
		})
	}
}

func TestOutputDescription(t *testing.T) {
	var out Output
	require.NotEmpty(t, out.Description())
}

func TestOutputStart(t *testing.T) {
	fs := afero.NewMemMapFs()

	out, err := New(output.Params{ConfigArgument: "test.out", FS: fs})
	require.NoError(t, err)

	err = out.Start()
	require.NoError(t, err)

	err = out.Stop()
	require.NoError(t, err)

	// At this point we should have an empty file.
	fi, err := fs.Stat("test.out")
	require.NoError(t, err)
	require.Equal(t, int64(0), fi.Size())
}

// TestOutputStop tests that the metrics are correctly collected and written to the file.
func TestOutputStop(t *testing.T) {
	fs := afero.NewMemMapFs()

	out, err := New(output.Params{ConfigArgument: "test.out", FS: fs})
	require.NoError(t, err)

	err = out.Start()
	require.NoError(t, err)

	// TODO(mem): add samples

	err = out.Stop()
	require.NoError(t, err)

	fi, err := fs.Stat("test.out")
	require.NoError(t, err)
	require.Equal(t, int64(0), fi.Size())
}

func makeSample(name string, value float64) metrics.Sample {
	return metrics.Sample{
		TimeSeries: metrics.TimeSeries{
			Metric: &metrics.Metric{
				Name: name,
			},
		},
		Value: value,
	}
}

func TestDeriveMetricNameAndValue(t *testing.T) {

	testcases := map[string]struct {
		input         metrics.Sample
		expectedName  string
		expectedValue float64
	}{
		"iterations": {
			input:         makeSample("iterations", 1),
			expectedName:  "",
			expectedValue: 0,
		},
		"checks": {
			input:         makeSample("checks", 1),
			expectedName:  "checks_total",
			expectedValue: 1,
		},
		"iteration_duration": {
			input:         makeSample("iteration_duration", 1),
			expectedName:  "iteration_duration_seconds",
			expectedValue: 0.001,
		},
		"data_sent": {
			input:         makeSample("data_sent", 1),
			expectedName:  "data_sent_bytes",
			expectedValue: 1,
		},
		"data_received": {
			input:         makeSample("data_received", 1),
			expectedName:  "data_received_bytes",
			expectedValue: 1,
		},
		"something_else": {
			input:         makeSample("something_else", 42),
			expectedName:  "something_else",
			expectedValue: 42,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actualName, actualValue := deriveMetricNameAndValue(tc.input)
			require.Equal(t, tc.expectedName, actualName)
			require.Equal(t, tc.expectedValue, actualValue)
		})
	}
}

func TestGetStats(t *testing.T) {
	testcases := map[string]struct {
		input    []float64
		expected stats
	}{
		"1": { // single sample
			input:    []float64{1.0},
			expected: stats{n: 1, min: 1, max: 1, sum: 1, med: 1},
		},
		"2": { // two samples
			input:    []float64{1.0, 2.0},
			expected: stats{n: 2, min: 1, max: 2, sum: 3, med: 1.5},
		},
		"3": { // three samples, regular
			input:    []float64{1.0, 2.0, 3.0},
			expected: stats{n: 3, min: 1, max: 3, sum: 6, med: 2.0},
		},
		"3b": { // three samples, irregular
			input:    []float64{1.0, 2.0, 4.0},
			expected: stats{n: 3, min: 1, max: 4, sum: 7, med: 2.0},
		},
		"4": { // four samples, irregular
			input:    []float64{1.0, 2.0, 4.0, 5.0},
			expected: stats{n: 4, min: 1, max: 5, sum: 12, med: 3.0},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := getStats(tc.input)
			if tc.expected != actual {
				t.Log("expected:", tc.expected, "actual:", actual)
				t.Fail()
			}
		})
	}
}

func TestIsValidMetricName(t *testing.T) {
	testcases := map[string]struct {
		input    string
		expected bool
	}{
		"single letter":         {input: "a", expected: true},
		"word":                  {input: "abc", expected: true},
		"letter and number":     {input: "a1", expected: true},
		"number":                {input: "1", expected: false},
		"numbers":               {input: "123", expected: false},
		"underscore":            {input: "_", expected: true},
		"valid with underscore": {input: "a_b_c", expected: true},
		"valid with numbers":    {input: "a_1_2", expected: true},
		"colon":                 {input: ":", expected: true},
		"namespace":             {input: "abc::xyz", expected: true},
		"blank":                 {input: " ", expected: false},
		"words with blank":      {input: "abc xyz", expected: false},
		"dash":                  {input: "-", expected: false},
		"words with dash":       {input: "abc-xyz", expected: false},
		"utf8":                  {input: "á", expected: false},
		"empty":                 {input: "", expected: false},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := isValidMetricNameRe(tc.input)
			if actual != tc.expected {
				t.Log("expected:", tc.expected, "actual:", actual, "input:", tc.input)
				t.Fail()
			}

			actualNonRe := isValidMetricName(tc.input)
			if actualNonRe != actual {
				t.Log("expected:", actual, "actual:", actualNonRe, "input:", tc.input)
				t.Fail()
			}
		})
	}
}

func TestSanitizeLabelName(t *testing.T) {
	testcases := map[string]struct {
		input    string
		expected string
	}{
		"single letter":         {input: "a", expected: "a"},
		"word":                  {input: "abc", expected: "abc"},
		"letter and number":     {input: "a1", expected: "a1"},
		"number":                {input: "1", expected: "_"},
		"numbers":               {input: "123", expected: "_23"},
		"underscore":            {input: "_", expected: "_"},
		"valid with underscore": {input: "a_b_c", expected: "a_b_c"},
		"valid with numbers":    {input: "a_1_2", expected: "a_1_2"},
		"colon":                 {input: ":", expected: ":"},
		"namespace":             {input: "abc::xyz", expected: "abc::xyz"},
		"blank":                 {input: " ", expected: "_"},
		"words with blank":      {input: "abc xyz", expected: "abc_xyz"},
		"dash":                  {input: "-", expected: "_"},
		"words with dash":       {input: "abc-xyz", expected: "abc_xyz"},
		"utf8":                  {input: "á", expected: "_"},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := sanitizeLabelName(tc.input)
			if actual != tc.expected {
				t.Log("expected:", tc.expected, "actual:", actual, "input:", tc.input)
				t.Fail()
			}
		})
	}
}

func TestBufferedMetricTextOutputValue(t *testing.T) {
	type metricData struct {
		name  string
		kvs   []string
		value float64
	}

	testcases := map[string]struct {
		kvs      []string
		data     []metricData
		expected string
	}{
		"basic": {
			data: []metricData{
				{
					name:  "test",
					value: 1,
				},
			},
			expected: "test{} 1\n",
		},
		"one keyval": {
			kvs: []string{"key", "value"},
			data: []metricData{
				{
					name:  "test",
					value: 1,
				},
			},
			expected: "test{key=\"value\"} 1\n",
		},
		"multiple keyval": {
			kvs: []string{"key1", "1", "key2", "2"},
			data: []metricData{
				{
					name:  "test",
					value: 1,
				},
			},
			expected: "test{key1=\"1\",key2=\"2\"} 1\n",
		},
		"extra keyvals": {
			kvs: []string{"key1", "1"},
			data: []metricData{
				{
					name:  "test",
					kvs:   []string{"key2", "2"},
					value: 1,
				},
			},
			expected: "test{key2=\"2\",key1=\"1\"} 1\n",
		},
		"invalid key": {
			kvs: []string{"key 1", "1", "key 2", "2"},
			data: []metricData{
				{
					name:  "test",
					value: 1,
				},
			},
			expected: "test{key_1=\"1\",key_2=\"2\"} 1\n",
		},
		"multiple metrics": {
			kvs: []string{"key 1", "1", "key 2", "2"},
			data: []metricData{
				{
					name:  "a",
					value: 1,
					kvs:   []string{"key3", "3"},
				},
				{
					name:  "b",
					value: 2,
					kvs:   []string{"key4", "4"},
				},
			},
			expected: "a{key3=\"3\",key_1=\"1\",key_2=\"2\"} 1\nb{key4=\"4\",key_1=\"1\",key_2=\"2\"} 2\n",
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			to := newBufferedMetricTextOutput(&buf, tc.kvs...)
			for _, d := range tc.data {
				to.Name(d.name)
				for i := 0; i < len(d.kvs); i += 2 {
					to.KeyValue(d.kvs[i], d.kvs[i+1])
				}
				to.Value(d.value)
			}
			require.Equal(t, tc.expected, buf.String())
		})
	}
}

func joinNewline(s ...string) string {
	return strings.Join(s, "\n") + "\n"
}

func TestBufferedMetricTextOutputStats(t *testing.T) {
	type metricData struct {
		name   string
		kvs    []string
		values []float64
	}

	testcases := map[string]struct {
		kvs      []string
		data     []metricData
		expected string
	}{
		"basic": {
			data: []metricData{
				{
					name:   "test",
					values: []float64{1},
				},
			},
			expected: joinNewline(
				`test_min{} 1`,
				`test_max{} 1`,
				`test{} 1`,
				`test_count{} 1`,
				`test_sum{} 1`,
			),
		},
		"one keyval": {
			kvs: []string{"key", "value"},
			data: []metricData{
				{
					name:   "test",
					values: []float64{1},
				},
			},
			expected: joinNewline(
				`test_min{key="value"} 1`,
				`test_max{key="value"} 1`,
				`test{key="value"} 1`,
				`test_count{key="value"} 1`,
				`test_sum{key="value"} 1`,
			),
		},
		"multiple keyval": {
			kvs: []string{"key1", "1", "key2", "2"},
			data: []metricData{
				{
					name:   "test",
					values: []float64{1},
				},
			},
			expected: joinNewline(
				`test_min{key1="1",key2="2"} 1`,
				`test_max{key1="1",key2="2"} 1`,
				`test{key1="1",key2="2"} 1`,
				`test_count{key1="1",key2="2"} 1`,
				`test_sum{key1="1",key2="2"} 1`,
			),
		},
		"extra keyvals": {
			kvs: []string{"key1", "1"},
			data: []metricData{
				{
					name:   "test",
					kvs:    []string{"key2", "2"},
					values: []float64{1},
				},
			},
			expected: joinNewline(
				`test_min{key2="2",key1="1"} 1`,
				`test_max{key2="2",key1="1"} 1`,
				`test{key2="2",key1="1"} 1`,
				`test_count{key2="2",key1="1"} 1`,
				`test_sum{key2="2",key1="1"} 1`,
			),
		},
		"invalid key": {
			kvs: []string{"key 1", "1", "key 2", "2"},
			data: []metricData{
				{
					name:   "test",
					values: []float64{1},
				},
			},
			expected: joinNewline(
				`test_min{key_1="1",key_2="2"} 1`,
				`test_max{key_1="1",key_2="2"} 1`,
				`test{key_1="1",key_2="2"} 1`,
				`test_count{key_1="1",key_2="2"} 1`,
				`test_sum{key_1="1",key_2="2"} 1`,
			),
		},
		"multiple metrics": {
			kvs: []string{"key 1", "1", "key 2", "2"},
			data: []metricData{
				{
					name:   "a",
					values: []float64{1, 2, 3},
					kvs:    []string{"key3", "3"},
				},
				{
					name:   "b",
					values: []float64{2, 4, 6},
					kvs:    []string{"key 4", "4", "key5", "5"},
				},
			},
			expected: joinNewline(
				`a_min{key3="3",key_1="1",key_2="2"} 1`,
				`a_max{key3="3",key_1="1",key_2="2"} 3`,
				`a{key3="3",key_1="1",key_2="2"} 2`,
				`a_count{key3="3",key_1="1",key_2="2"} 3`,
				`a_sum{key3="3",key_1="1",key_2="2"} 6`,
				`b_min{key_4="4",key5="5",key_1="1",key_2="2"} 2`,
				`b_max{key_4="4",key5="5",key_1="1",key_2="2"} 6`,
				`b{key_4="4",key5="5",key_1="1",key_2="2"} 4`,
				`b_count{key_4="4",key5="5",key_1="1",key_2="2"} 3`,
				`b_sum{key_4="4",key5="5",key_1="1",key_2="2"} 12`,
			),
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			to := newBufferedMetricTextOutput(&buf, tc.kvs...)
			for _, d := range tc.data {
				to.Name(d.name)
				for i := 0; i < len(d.kvs); i += 2 {
					to.KeyValue(d.kvs[i], d.kvs[i+1])
				}
				to.Stats(d.values)
			}
			require.Equal(t, tc.expected, buf.String())
		})
	}
}

func TestTargetMetricsCollectionWriteOne(t *testing.T) {
	c := newTargetMetricsCollection()

	require.Len(t, c, 0)

	c[targetId{
		url:      "http://example.com",
		method:   "GET",
		scenario: "s",
		group:    "g",
	}] = targetMetrics{
		requests:         1,
		failed:           0,
		expectedResponse: false,
		scenario:         "s",
		group:            "g",
		proto:            "1.1",
		tlsVersion:       "1.3",
		status:           []string{"200"},
		duration:         []float64{0.001},
		blocked:          []float64{0.001},
		connecting:       []float64{0.001},
		sending:          []float64{0.001},
		waiting:          []float64{0.001},
		receiving:        []float64{0.001},
		tlsHandshaking:   []float64{0.001},
		tags:             map[string]string{"k": "v"},
	}

	var buf bytes.Buffer

	c.Write(&buf)

	expected := joinNewline(
		`probe_http_info{tls_version="1.3",proto="1.1",k="v",url="http://example.com",method="GET",scenario="s",group="g"} 1`,
		`probe_http_requests_total{url="http://example.com",method="GET",scenario="s",group="g"} 1`,
		`probe_http_requests_failed_total{url="http://example.com",method="GET",scenario="s",group="g"} 0`,
		`probe_http_status_code{url="http://example.com",method="GET",scenario="s",group="g"} 200`,
		`probe_http_version{url="http://example.com",method="GET",scenario="s",group="g"} 1.1`,
		`probe_http_ssl{url="http://example.com",method="GET",scenario="s",group="g"} 1`,
		`probe_http_duration_seconds{phase="resolve",url="http://example.com",method="GET",scenario="s",group="g"} 0`,
		`probe_http_duration_seconds{phase="connect",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds{phase="tls",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds{phase="processing",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds{phase="transfer",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
	)

	require.Equal(t, expected, buf.String())
}

func TestTargetMetricsCollectionWriteMany(t *testing.T) {
	c := newTargetMetricsCollection()

	require.Len(t, c, 0)

	c[targetId{
		url:      "http://example.com",
		method:   "GET",
		scenario: "s",
		group:    "g",
	}] = targetMetrics{
		requests:         2,
		failed:           1,
		expectedResponse: false,
		scenario:         "s",
		group:            "g",
		proto:            "1.1",
		tlsVersion:       "1.3",
		status:           []string{"200", "200"},
		duration:         []float64{0.001, 0.001},
		blocked:          []float64{0.001, 0.001},
		connecting:       []float64{0.001, 0.001},
		sending:          []float64{0.001, 0.001},
		waiting:          []float64{0.001, 0.001},
		receiving:        []float64{0.001, 0.001},
		tlsHandshaking:   []float64{0.001, 0.001},
		tags:             map[string]string{"k": "v"},
	}

	var buf bytes.Buffer

	c.Write(&buf)

	expected := joinNewline(
		`probe_http_info{tls_version="1.3",proto="1.1",k="v",url="http://example.com",method="GET",scenario="s",group="g"} 1`,
		`probe_http_requests_total{url="http://example.com",method="GET",scenario="s",group="g"} 2`,
		`probe_http_requests_failed_total{url="http://example.com",method="GET",scenario="s",group="g"} 1`,
		`probe_http_status_code{url="http://example.com",method="GET",scenario="s",group="g"} 200`,
		`probe_http_version{url="http://example.com",method="GET",scenario="s",group="g"} 1.1`,
		`probe_http_ssl{url="http://example.com",method="GET",scenario="s",group="g"} 1`,
		`probe_http_duration_seconds_min{phase="resolve",url="http://example.com",method="GET",scenario="s",group="g"} 0`,
		`probe_http_duration_seconds_max{phase="resolve",url="http://example.com",method="GET",scenario="s",group="g"} 0`,
		`probe_http_duration_seconds{phase="resolve",url="http://example.com",method="GET",scenario="s",group="g"} 0`,
		`probe_http_duration_seconds_count{phase="resolve",url="http://example.com",method="GET",scenario="s",group="g"} 2`,
		`probe_http_duration_seconds_sum{phase="resolve",url="http://example.com",method="GET",scenario="s",group="g"} 0`,
		`probe_http_duration_seconds_min{phase="connect",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds_max{phase="connect",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds{phase="connect",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds_count{phase="connect",url="http://example.com",method="GET",scenario="s",group="g"} 2`,
		`probe_http_duration_seconds_sum{phase="connect",url="http://example.com",method="GET",scenario="s",group="g"} 0.002`,
		`probe_http_duration_seconds_min{phase="tls",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds_max{phase="tls",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds{phase="tls",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds_count{phase="tls",url="http://example.com",method="GET",scenario="s",group="g"} 2`,
		`probe_http_duration_seconds_sum{phase="tls",url="http://example.com",method="GET",scenario="s",group="g"} 0.002`,
		`probe_http_duration_seconds_min{phase="processing",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds_max{phase="processing",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds{phase="processing",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds_count{phase="processing",url="http://example.com",method="GET",scenario="s",group="g"} 2`,
		`probe_http_duration_seconds_sum{phase="processing",url="http://example.com",method="GET",scenario="s",group="g"} 0.002`,
		`probe_http_duration_seconds_min{phase="transfer",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds_max{phase="transfer",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds{phase="transfer",url="http://example.com",method="GET",scenario="s",group="g"} 0.001`,
		`probe_http_duration_seconds_count{phase="transfer",url="http://example.com",method="GET",scenario="s",group="g"} 2`,
		`probe_http_duration_seconds_sum{phase="transfer",url="http://example.com",method="GET",scenario="s",group="g"} 0.002`,
	)

	require.Equal(t, expected, buf.String())
}
