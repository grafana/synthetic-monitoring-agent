package k6runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-logfmt/logfmt"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	r1 := New(RunnerOpts{Uri: "k6"})
	require.IsType(t, LocalRunner{}, r1)
	require.Equal(t, "", r1.(LocalRunner).blacklistedIP)

	r2 := New(RunnerOpts{Uri: "/usr/bin/k6", BlacklistedIP: "192.168.4.0/24"})
	require.IsType(t, LocalRunner{}, r2)
	require.Equal(t, "192.168.4.0/24", r2.(LocalRunner).blacklistedIP)
	// Ensure WithLogger preserves config.
	zl := zerolog.New(io.Discard)
	r2 = r2.WithLogger(&zl)
	require.Equal(t, "192.168.4.0/24", r2.(LocalRunner).blacklistedIP)

	r3 := New(RunnerOpts{Uri: "http://localhost:6565"})
	require.IsType(t, &HttpRunner{}, r3)
	r4 := New(RunnerOpts{Uri: "https://localhost:6565"})
	require.IsType(t, &HttpRunner{}, r4)
}

func TestNewScript(t *testing.T) {
	runner := New(RunnerOpts{Uri: "k6"})
	script := Script{
		Script: []byte("test"),
		Settings: Settings{
			Timeout: 1000,
		},
	}

	processor, err := NewProcessor(script, runner)
	require.NoError(t, err)
	require.NotNil(t, processor)
	require.Equal(t, script, processor.script)
	require.Equal(t, runner, processor.runner)
}

func TestScriptRun(t *testing.T) {
	runner := testRunner{
		metrics: testhelper.MustReadFile(t, "testdata/test.out"),
		logs:    testhelper.MustReadFile(t, "testdata/test.log"),
	}

	processor, err := NewProcessor(Script{
		Script: testhelper.MustReadFile(t, "testdata/test.js"),
		Settings: Settings{
			Timeout: 1000,
		},
	}, &runner)
	require.NoError(t, err)
	require.NotNil(t, processor)

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
	success, err := processor.Run(ctx, registry, &logger, zlogger)
	require.NoError(t, err)
	require.True(t, success)
}

func TestHttpRunnerRun(t *testing.T) {
	script := Script{
		Script: testhelper.MustReadFile(t, "testdata/test.js"),
		Settings: Settings{
			Timeout: 1000,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req Script
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		require.Equal(t, script, req)

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

	runner := New(RunnerOpts{Uri: srv.URL + "/run"})
	require.IsType(t, &HttpRunner{}, runner)

	ctx := context.Background()
	ctx, cancel := testhelper.Context(ctx, t)
	t.Cleanup(cancel)

	_, err := runner.Run(ctx, script)
	require.NoError(t, err)
}

func TestHttpRunnerRunError(t *testing.T) {
	script := Script{
		Script: testhelper.MustReadFile(t, "testdata/test.js"),
		Settings: Settings{
			Timeout: 1000,
		},
	}

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
		t.Log("http runner called the wrong endpoint")
		t.Fail()

		w.WriteHeader(http.StatusBadRequest)
		resp := requestError{
			Err:     http.StatusText(http.StatusBadRequest),
			Message: "unexpected request to " + r.URL.Path,
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	runner := New(RunnerOpts{Uri: srv.URL + "/run"})
	require.IsType(t, &HttpRunner{}, runner)

	ctx, cancel := testhelper.Context(context.Background(), t)
	t.Cleanup(cancel)

	_, err := runner.Run(ctx, script)
	require.Error(t, err)
}

// TestScriptHTTPRun tests that Script reports what it should depending on the status code and responses of the HTTP
// runner.
func TestScriptHTTPRun(t *testing.T) {
	t.Parallel()

	var (
		testMetrics = testhelper.MustReadFile(t, "testdata/test.out")
		// testLogs is a file containing a bunch of log lines. All lines but one have level=debug, which are discarded
		// by the loki submitter implementation.
		testLogs = testhelper.MustReadFile(t, "testdata/test.log")
		// nonkdebugLogLine is the only line on testLogs that does not have level=debug, therefore the only one that is
		// actually submitted. We use it in the test table below as a sentinel to assert whether logs have been
		// submitted or not.
		nonDebugLogLine = `time="2023-06-01T13:40:26-06:00" level="test" msg="Non-debug message, for testing!"` + "\n"
	)

	for _, tc := range []struct {
		name          string
		response      *RunResponse
		delay         time.Duration
		statusCode    int
		expectSuccess bool
		expectError   error
		expectErrorAs any // To accommodate return of unnamed errors. If set, expectError is ignored.
		expectLogs    string
	}{
		{
			name: "all good",
			response: &RunResponse{
				Metrics: testMetrics,
				Logs:    testLogs,
			},
			statusCode:    http.StatusOK,
			expectSuccess: true,
			expectError:   nil,
			expectLogs:    nonDebugLogLine,
		},
		{
			// HTTP runner should report failure and an error when the upstream status is not recognized.
			name:          "unexpected status",
			response:      &RunResponse{},
			statusCode:    999,
			expectSuccess: false,
			expectError:   ErrUnexpectedStatus,
		},
		{
			// HTTP runner should report failure and an error when the error is unknown.
			name: "non-user error",
			response: &RunResponse{
				Metrics:   testMetrics,
				Logs:      testLogs,
				Error:     "something went wrong",
				ErrorCode: "something-wrong",
			},
			statusCode:    http.StatusOK,
			expectSuccess: false,
			expectError:   ErrFromRunner,
			expectLogs:    nonDebugLogLine,
		},
		{
			// HTTP runner should report failure but no error when the error is unknown.
			name: "user error",
			response: &RunResponse{
				Metrics:   testMetrics,
				Logs:      testLogs,
				Error:     "syntax error somewhere or something",
				ErrorCode: "user",
			},
			statusCode:    http.StatusOK,
			expectSuccess: false,
			expectError:   nil,
			expectLogs:    nonDebugLogLine,
		},
		{
			name: "borked logs are sent best-effort",
			response: &RunResponse{
				Metrics:   testMetrics,
				Logs:      []byte(`level=error foo="b` + "\n"),
				Error:     "we killed k6",
				ErrorCode: "user",
			},
			statusCode:    http.StatusUnprocessableEntity,
			expectSuccess: false,
			expectErrorAs: &logfmt.SyntaxError{},
			expectLogs:    `level="error"` + "\n",
		},
		{
			name: "logs are sent on borked metrics",
			response: &RunResponse{
				Metrics:   []byte("probe_succ{"),
				Logs:      testLogs,
				Error:     "we killed k6",
				ErrorCode: "user",
			},
			statusCode:    http.StatusUnprocessableEntity,
			expectSuccess: false,
			expectErrorAs: expfmt.ParseError{},
			expectLogs:    nonDebugLogLine,
		},
		{
			name: "inconsistent runner response A",
			response: &RunResponse{
				Metrics:   testMetrics,
				Logs:      testLogs,
				Error:     "set",
				ErrorCode: "",
			},
			statusCode:    http.StatusInternalServerError,
			expectSuccess: false,
			expectError:   ErrBuggyRunner,
			expectLogs:    nonDebugLogLine,
		},
		{
			name: "inconsistent runner response B",
			response: &RunResponse{
				Metrics:   testMetrics,
				Logs:      testLogs,
				Error:     "",
				ErrorCode: "set",
			},
			statusCode:    http.StatusInternalServerError,
			expectSuccess: false,
			expectError:   ErrBuggyRunner,
			expectLogs:    nonDebugLogLine,
		},
		{
			name: "request timeout",
			response: &RunResponse{
				Metrics: testMetrics,
				Logs:    testLogs,
			},
			delay:         2 * time.Second,
			statusCode:    http.StatusInternalServerError,
			expectSuccess: false,
			expectError:   context.DeadlineExceeded,
		},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()
			mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tc.delay)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_ = json.NewEncoder(w).Encode(tc.response)
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			runner := New(RunnerOpts{Uri: srv.URL + "/run"})
			script, err := NewProcessor(Script{
				Script: []byte("tee-hee"),
				Settings: Settings{
					Timeout: 1000,
				},
			}, runner)
			require.NoError(t, err)

			baseCtx, baseCancel := context.WithTimeout(context.Background(), time.Second)
			t.Cleanup(baseCancel)
			ctx, cancel := testhelper.Context(baseCtx, t)
			t.Cleanup(cancel)

			var (
				registry = prometheus.NewRegistry()
				logBuf   = bytes.Buffer{}
				logger   = recordingLogger{buf: &logBuf}
				zlogger  = zerolog.Nop()
			)

			success, err := script.Run(ctx, registry, logger, zlogger)
			require.Equal(t, tc.expectSuccess, success)
			require.Equal(t, tc.expectLogs, logger.buf.String())
			if tc.expectErrorAs == nil {
				require.ErrorIs(t, err, tc.expectError)
			} else {
				require.ErrorAs(t, err, &tc.expectErrorAs)
			}
		})
	}
}

type testRunner struct {
	metrics []byte
	logs    []byte
}

var _ Runner = &testRunner{}

func (r *testRunner) Run(ctx context.Context, script Script) (*RunResponse, error) {
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

	promDecoder := expfmt.NewDecoder(bytes.NewBuffer(data), expfmt.NewFormat(expfmt.TypeTextPlain))
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

	var sampleCollector sampleCollector

	buf := bytes.Buffer{}
	logger := zerolog.New(&buf)

	err := extractMetricSamples(data, logger, sampleCollector.process)
	require.NoError(t, err)

	registry := prometheus.NewRegistry()
	err = registry.Register(&sampleCollector.collector)
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

type testLogger struct{}

var _ logger.Logger = &testLogger{}

func (l *testLogger) Log(keyvals ...any) error {
	if len(keyvals) == 0 {
		return errors.New("empty log message")
	}
	return nil
}

type recordingLogger struct {
	buf *bytes.Buffer
}

var _ logger.Logger = &recordingLogger{}

func (l recordingLogger) Log(keyvals ...any) error {
	if len(keyvals) == 0 {
		return errors.New("empty log message")
	}

	if len(keyvals)%2 != 0 {
		return errors.New("not the same number of keys and vals")
	}

	line := make([]string, 0, len(keyvals)/2)
	for i := 0; i < len(keyvals); i += 2 {
		key := keyvals[i]
		val := keyvals[i+1]

		line = append(line, fmt.Sprintf("%s=%q", key, val))
	}

	_, err := fmt.Fprintln(l.buf, strings.Join(line, " "))
	return err
}
