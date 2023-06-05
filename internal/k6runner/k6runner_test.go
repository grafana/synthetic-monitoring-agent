package k6runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	r1 := New("k6")
	require.IsType(t, &LocalRunner{}, r1)
	r2 := New("/usr/bin/k6")
	require.IsType(t, &LocalRunner{}, r2)
	r3 := New("http://localhost:6565")
	require.IsType(t, &HttpRunner{}, r3)
	r4 := New("https://localhost:6565")
	require.IsType(t, &HttpRunner{}, r4)
}

func TestNewScript(t *testing.T) {
	runner := New("k6")
	src := []byte("test")
	script, err := NewScript(src, runner)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.Equal(t, src, script.script)
	require.Equal(t, runner, script.runner)
}

func TestScriptRun(t *testing.T) {
	runner := testRunner{
		metrics: testhelper.MustReadFile(t, "testdata/test.out"),
		logs:    testhelper.MustReadFile(t, "testdata/test.log"),
	}

	script, err := NewScript(testhelper.MustReadFile(t, "testdata/test.js"), &runner)
	require.NoError(t, err)
	require.NotNil(t, script)

	var (
		registry = prometheus.NewRegistry()
		logger   testLogger
		buf      bytes.Buffer
		zlogger  = zerolog.New(&buf)
	)

	ctx, cancel := testhelper.Context(context.Background(), t)
	t.Cleanup(cancel)

	// We already know tha parsing the metrics and the logs is working, so
	// we are only interested in verifying that the script runs without
	// errors.
	err = script.Run(ctx, registry, &logger, zlogger)
	require.NoError(t, err)
}

func TestHttpRunnerRun(t *testing.T) {
	scriptSrc := testhelper.MustReadFile(t, "testdata/test.js")

	mux := http.NewServeMux()
	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req RunRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		require.Equal(t, scriptSrc, req.Script)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		rr := &RunResponse{
			Metrics: testhelper.MustReadFile(t, "testdata/test.out"),
			Logs:    testhelper.MustReadFile(t, "testdata/test.log"),
		}
		_ = json.NewEncoder(w).Encode(rr)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"msg": "bad request"}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	runner := New(srv.URL + "/run")
	require.IsType(t, &HttpRunner{}, runner)

	ctx, cancel := testhelper.Context(context.Background(), t)
	t.Cleanup(cancel)

	_, err := runner.Run(ctx, scriptSrc)
	require.NoError(t, err)
}

func TestHttpRunnerRunError(t *testing.T) {
	scriptSrc := testhelper.MustReadFile(t, "testdata/test.js")

	mux := http.NewServeMux()
	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		resp := requestError{
			Err:     http.StatusText(http.StatusBadRequest),
			Message: "test error",
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := requestError{
			Err:     http.StatusText(http.StatusBadRequest),
			Message: "unexpected request to " + r.URL.Path,
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	runner := New(srv.URL + "/run")
	require.IsType(t, &HttpRunner{}, runner)

	ctx, cancel := testhelper.Context(context.Background(), t)
	t.Cleanup(cancel)

	_, err := runner.Run(ctx, scriptSrc)
	require.Error(t, err)
}

type testRunner struct {
	metrics []byte
	logs    []byte
}

var _ Runner = &testRunner{}

func (r *testRunner) Run(ctx context.Context, script []byte) (*RunResponse, error) {
	return &RunResponse{
		Metrics: r.metrics,
		Logs:    r.logs,
	}, nil
}

func (r *testRunner) WithLogger(logger *zerolog.Logger) Runner {
	return r
}

func TestTextToRegistry(t *testing.T) {
	data := testhelper.MustReadFile(t, "testdata/test.out")

	expectedMetrics := map[string]struct{}{}

	promDecoder := expfmt.NewDecoder(bytes.NewBuffer(data), expfmt.FmtText)
DEC_LOOP:
	for {
		var mf dto.MetricFamily
		switch err := promDecoder.Decode(&mf); err {
		case nil:
			for _, m := range mf.GetMetric() {
				expectedMetrics[buildId(mf.GetName(), m)] = struct{}{}
			}
		case io.EOF:
			break DEC_LOOP
		default:
			t.Fatal(err)
		}
	}

	registry := prometheus.NewRegistry()
	buf := bytes.Buffer{}
	logger := zerolog.New(&buf)
	err := textToRegistry(data, registry, logger)
	require.NoError(t, err)

	mfs, err := registry.Gather()
	require.NoError(t, err)

	actualMetrics := map[string]struct{}{}
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			actualMetrics[buildId(mf.GetName(), m)] = struct{}{}
		}
	}

	// This is some minimal validation that all the metrics are parsed and
	// added to the registry.
	require.Equal(t, expectedMetrics, actualMetrics)
}

func buildId(name string, m *dto.Metric) string {
	labels := m.GetLabel()
	sort.Slice(labels, func(i, j int) bool {
		switch rel := strings.Compare(labels[i].GetName(), labels[j].GetName()); rel {
		case 0:
			return strings.Compare(labels[i].GetValue(), labels[j].GetValue()) < 0
		default:
			return rel < 0
		}
	})

	var s strings.Builder
	s.WriteString(name)
	for _, l := range labels {
		s.WriteString(",")
		s.WriteString(l.GetName())
		s.WriteString("=")
		s.WriteString(l.GetValue())
	}

	return s.String()
}

func TestK6LogsToLogger(t *testing.T) {
	data := testhelper.MustReadFile(t, "testdata/test.log")

	var logger testLogger

	err := k6LogsToLogger(data, &logger)
	require.NoError(t, err)
}

type testLogger struct {
}

var _ logger.Logger = &testLogger{}

func (l *testLogger) Log(keyvals ...any) error {
	if len(keyvals) == 0 {
		return errors.New("empty log message")
	}
	return nil
}
