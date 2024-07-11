package k6runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
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

	// HTTPRunner will retry until the context deadline is met, so we set a short one.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	ctx, cancel = testhelper.Context(ctx, t)
	t.Cleanup(cancel)

	_, err := runner.Run(ctx, script)
	require.ErrorIs(t, err, ErrUnexpectedStatus)
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
			delay:         3 * time.Second, // Beyond timeout + graceTime.
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

			runner := HttpRunner{url: srv.URL + "/run", graceTime: time.Second, backoff: time.Second}
			script, err := NewProcessor(Script{
				Script: []byte("tee-hee"),
				Settings: Settings{
					Timeout: 1000,
				},
			}, runner)
			require.NoError(t, err)

			baseCtx, baseCancel := context.WithTimeout(context.Background(), 4*time.Second)
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

func TestHTTPProcessorRetries(t *testing.T) {
	t.Parallel()

	t.Run("status codes", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name           string
			handler        http.Handler
			scriptTimeout  time.Duration
			graceTime      time.Duration
			globalTimeout  time.Duration
			expectRequests int64
			expectError    error
		}{
			{
				name:          "no retries needed",
				handler:       emptyJSON(http.StatusOK),
				scriptTimeout: time.Second, graceTime: time.Second, globalTimeout: 5 * time.Second,
				expectRequests: 1,
				expectError:    nil,
			},
			{
				name:          "does not retry 400",
				handler:       emptyJSON(http.StatusBadRequest),
				scriptTimeout: time.Second, graceTime: time.Second, globalTimeout: 5 * time.Second,
				expectRequests: 1,
				expectError:    ErrUnexpectedStatus,
			},
			{
				name:          "does not retry 422",
				handler:       emptyJSON(http.StatusUnprocessableEntity),
				scriptTimeout: time.Second, graceTime: time.Second, globalTimeout: 5 * time.Second,
				expectRequests: 1,
				expectError:    nil,
			},
			{
				name:          "does not retry 428",
				handler:       emptyJSON(http.StatusRequestTimeout),
				scriptTimeout: time.Second, graceTime: time.Second, globalTimeout: 5 * time.Second,
				expectRequests: 1,
				expectError:    nil,
			},
			{
				name:          "does not retry 500",
				handler:       emptyJSON(http.StatusInternalServerError),
				scriptTimeout: time.Second, graceTime: time.Second, globalTimeout: 5 * time.Second,
				expectRequests: 1,
				expectError:    nil,
			},
			{
				name:          "retries 503",
				handler:       afterAttempts(emptyJSON(http.StatusServiceUnavailable), 1, emptyJSON(http.StatusOK)),
				scriptTimeout: time.Second, graceTime: time.Second, globalTimeout: 5 * time.Second,
				expectRequests: 2,
				expectError:    nil,
			},
			{
				name:          "retries 504",
				handler:       afterAttempts(emptyJSON(http.StatusGatewayTimeout), 1, emptyJSON(http.StatusOK)),
				scriptTimeout: time.Second, graceTime: time.Second, globalTimeout: 5 * time.Second,
				expectRequests: 2,
				expectError:    nil,
			},
			{
				name:          "retries more than once",
				handler:       afterAttempts(emptyJSON(http.StatusGatewayTimeout), 2, emptyJSON(http.StatusOK)),
				scriptTimeout: time.Second, graceTime: time.Second, globalTimeout: 5 * time.Second,
				expectRequests: 3,
				expectError:    nil,
			},
			{
				name:          "gives up eventually",
				handler:       emptyJSON(http.StatusServiceUnavailable),
				scriptTimeout: time.Second, graceTime: time.Second, globalTimeout: 5 * time.Second,
				// Context is forced to timeout after 5 seconds. This means 3 requests, with delays of 0, [1-2), [2-3), as
				// the fourth request would need to wait [3-4) seconds after [3-5) have passed, thus guaranteed to
				// go beyond the 5s deadline.
				expectRequests: 3,
				expectError:    context.DeadlineExceeded,
			},
			{
				name:          "gives up eventually when server hangs",
				handler:       delay(10*time.Second, emptyJSON(http.StatusServiceUnavailable)),
				scriptTimeout: time.Second, graceTime: time.Second, globalTimeout: 5 * time.Second,
				// Requests have 2s timeout and 0s backoff after that (backoff includes timeout in
				// this implementation), so that means requests are made at the 0s, 2s and 4s marks. A fourth request
				// would happen beyond the 5s timeline.
				expectRequests: 3,
				expectError:    context.DeadlineExceeded,
			},
		} {
			tc := tc

			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Use atomic as we write to this in a handler and read from in on the test.
				// go test -race trips if we use a regular int here.
				var requests atomic.Int64

				mux := http.NewServeMux()
				mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					tc.handler.ServeHTTP(w, r)
				})
				srv := httptest.NewServer(mux)
				t.Cleanup(srv.Close)

				runner := HttpRunner{url: srv.URL + "/run", graceTime: tc.graceTime, backoff: time.Second}
				processor, err := NewProcessor(Script{Script: nil, Settings: Settings{tc.scriptTimeout.Milliseconds()}}, runner)
				require.NoError(t, err)

				baseCtx, baseCancel := context.WithTimeout(context.Background(), tc.globalTimeout)
				t.Cleanup(baseCancel)
				ctx, cancel := testhelper.Context(baseCtx, t)
				t.Cleanup(cancel)

				var (
					registry = prometheus.NewRegistry()
					logger   testLogger
					zlogger  = zerolog.New(io.Discard)
				)
				success, err := processor.Run(ctx, registry, &logger, zlogger)
				require.ErrorIs(t, err, tc.expectError)
				require.Equal(t, tc.expectError == nil, success)
				require.Equal(t, tc.expectRequests, requests.Load())
			})
		}
	})

	t.Run("retries network errors", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.Handle("/run", emptyJSON(http.StatusOK))

		// TODO: Hand-picking a random port instead of letting the OS allocate one is terrible practice. However,
		// I haven't found a way to do this if we really want to know the address before something is listening on it.
		addr := net.JoinHostPort("localhost", strconv.Itoa(30000+rand.Intn(35535)))
		go func() {
			time.Sleep(time.Second)

			listener, err := net.Listen("tcp4", addr)
			if err != nil {
				t.Logf("failed to set up listener in a random port. You were really unlucky, run the test again. %v", err)
				t.Fail()
			}

			err = http.Serve(listener, mux)
			require.NoError(t, err)
			t.Cleanup(func() {
				listener.Close()
			})
		}()

		runner := HttpRunner{url: "http://" + addr + "/run", graceTime: time.Second, backoff: time.Second}
		processor, err := NewProcessor(Script{Script: nil, Settings: Settings{Timeout: 1000}}, runner)
		require.NoError(t, err)

		baseCtx, baseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.Cleanup(baseCancel)
		ctx, cancel := testhelper.Context(baseCtx, t)
		t.Cleanup(cancel)

		var (
			registry = prometheus.NewRegistry()
			logger   testLogger
			zlogger  = zerolog.New(io.Discard)
		)
		success, err := processor.Run(ctx, registry, &logger, zlogger)
		require.NoError(t, err)
		require.True(t, success)
	})
}

func emptyJSON(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte("{}"))
	})
}

// afterAttempts invokes the first handler if strictly less than [afterAttempts] requests have been made before to it.
// afterAttempts(a, 0, b) always invokes b.
// afterAttempts(a, 1, b) always invokes a for the first request, then b for the subsequent ones.
func afterAttempts(a http.Handler, afterAttempts int, b http.Handler) http.Handler {
	pastAttempts := 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pastAttempts >= afterAttempts {
			b.ServeHTTP(w, r)
			return
		}

		a.ServeHTTP(w, r)
		pastAttempts++
	})
}

// delay calls next after some time has passed. It watches for incoming request context cancellation to avoid leaking
// connections.
func delay(d time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fully consume the request body before waiting. If we do not do this, cancelling the request context will not
		// close the network connection, and httptest.Server will complain.
		_, _ = io.Copy(io.Discard, r.Body)

		select {
		case <-r.Context().Done():
			// Abort waiting if the client closes the request. Again, if we don't, httptest.Server will complain.
		case <-time.After(d):
			next.ServeHTTP(w, r)
		}
	})
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
