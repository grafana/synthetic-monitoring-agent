package k6runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logfmt/logfmt"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestHttpRunnerRun(t *testing.T) {
	t.Parallel()

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

		var req struct {
			Script   []byte    `json:"script"`
			Settings Settings  `json:"settings"`
			NotAfter time.Time `json:"notAfter"`
		}

		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Logf("decoding body: %v", err)
			t.Fail()
			w.WriteHeader(400) // Use 400 as the client won't retry this failure.
			return
		}

		if time.Since(req.NotAfter) > time.Hour || time.Until(req.NotAfter) > time.Hour {
			t.Log("unexpected value for NotAfter too far from the present")
			t.Fail()
			w.WriteHeader(400) // Use 400 as the client won't retry this failure.
			return
		}

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

	_, err := runner.Run(ctx, script, SecretStore{})
	require.NoError(t, err)
}

func TestHttpRunnerRunError(t *testing.T) {
	t.Parallel()

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

	_, err := runner.Run(ctx, script, SecretStore{})
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
		nonDebugLogLine = `{"level":"info","time":"2025-09-09T10:20:01Z","time":"2023-06-01T13:40:26-06:00","level":"test","message":"Non-debug message, for testing!"}` + "\n"
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
			expectLogs:    `{"level":"warn","error":"retryable\nunexpected status code 999","after":997.365291,"message":"retrying retryable error"}` + "\n" +
				`{"level":"warn","error":"retryable\nunexpected status code 999","after":2819.742557,"message":"retrying retryable error"}` + "\n" +
				`{"level":"warn","error":"retryable\nunexpected status code 999","after":4263.609863,"message":"retrying retryable error"}` + "\n" +
				`{"level":"error","error":"retryable\nunexpected status code 999","message":"retries exhausted"}` + "\n" +
				`{"level":"debug","error":"cannot retry further: retryable\nunexpected status code 999\ncontext deadline exceeded","message":"k6 script exited with error code"}` + "\n",
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
			expectLogs: nonDebugLogLine + fmt.Sprintf(
				`{"level":"error","time":"2025-09-09T10:20:01Z","error":%q,"errorCode":%q,"message":"script did not execute successfully"}` + "\n",
				"something went wrong",
				"something-wrong",
			),
		},
		{
			// HTTP runner should report failure but no error when the error is unknown.
			name: "user error",
			response: &RunResponse{
				Metrics:   testMetrics,
				Logs:      testLogs,
				Error:     "syntax error somewhere or something",
				ErrorCode: "aborted",
			},
			statusCode:    http.StatusOK,
			expectSuccess: false,
			expectError:   nil,
			expectLogs: nonDebugLogLine + fmt.Sprintf(
				`{"level":"error","time":"2025-09-09T10:20:01Z","error":%q,"errorCode":%q,"message":"script did not execute successfully"}` + "\n",
				"syntax error somewhere or something",
				"aborted",
			),
		},
		{
			name: "borked logs are sent best-effort",
			response: &RunResponse{
				Metrics:   testMetrics,
				Logs:      []byte(`level=error foo="b` + "\n"),
				Error:     "we killed k6",
				ErrorCode: "aborted",
			},
			statusCode:    http.StatusUnprocessableEntity,
			expectSuccess: false,
			expectErrorAs: &logfmt.SyntaxError{},
			expectLogs: `{"level":"error","time":"2025-09-09T10:20:01Z","level":"error"}` + "\n" + fmt.Sprintf(
				`{"level":"debug","error":"logfmt syntax error at pos 19 on line 1: unterminated quoted value","message":"cannot load logs to logger"}` + "\n" +
				`{"level":"error","time":"2025-09-09T10:20:01Z","error":%q,"errorCode":%q,"message":"script did not execute successfully"}` + "\n",
				"we killed k6",
				"aborted",
			),
		},
		{
			name: "logs are sent on borked metrics",
			response: &RunResponse{
				Metrics:   []byte("probe_succ{"),
				Logs:      testLogs,
				Error:     "we killed k6",
				ErrorCode: "aborted",
			},
			statusCode:    http.StatusUnprocessableEntity,
			expectSuccess: false,
			expectErrorAs: expfmt.ParseError{},
			expectLogs: nonDebugLogLine + fmt.Sprintf(
				`{"level":"error","error":"text format parsing error in line 1: unexpected end of input stream","message":"decoding prometheus metrics"}` + "\n" +
				`{"level":"debug","error":"decoding prometheus metrics: text format parsing error in line 1: unexpected end of input stream","message":"cannot extract metric samples"}` + "\n" +
				`{"level":"error","time":"2025-09-09T10:20:01Z","error":%q,"errorCode":%q,"message":"script did not execute successfully"}` + "\n",
				"we killed k6",
				"aborted",
			),
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
			expectLogs:    `{"level":"debug","metrics":"probe_data_sent_bytes{scenario=\"default\"} 0\nprobe_data_received_bytes{scenario=\"default\"} 0\nprobe_iteration_duration_seconds{scenario=\"default\"} 1.3029e-05\n","logs":"time=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Logger format: TEXT\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"k6 version: v0.43.1 ((devel), go1.20.4, linux/amd64)\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Resolving and reading test 'internal/k6runner/testdata/test.js'...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=Loading... moduleSpecifier=\"file:///home/marcelo/devel/grafana/sm-agent/internal/k6runner/testdata/test.js\" originalModuleSpecifier=internal/k6runner/testdata/test.js\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"'internal/k6runner/testdata/test.js' resolved to 'file:///home/marcelo/devel/grafana/sm-agent/internal/k6runner/testdata/test.js' and successfully loaded 31 bytes!\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Gathering k6 runtime options...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Initializing k6 runner for 'internal/k6runner/testdata/test.js' (file:///home/marcelo/devel/grafana/sm-agent/internal/k6runner/testdata/test.js)...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Detecting test type for...\" test_path=\"file:///home/marcelo/devel/grafana/sm-agent/internal/k6runner/testdata/test.js\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Trying to load as a JS test...\" test_path=\"file:///home/marcelo/devel/grafana/sm-agent/internal/k6runner/testdata/test.js\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Babel: Transformed\" t=8.993963ms\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Runner successfully initialized!\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Parsing CLI flags...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Consolidating config layers...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Parsing thresholds and validating config...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Initializing the execution scheduler...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Starting 2 outputs...\" component=output-manager\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=Starting... component=metrics-engine-ingester\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Started!\" component=metrics-engine-ingester\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Trapping interrupt signals so k6 can handle them gracefully...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Starting the REST API server on localhost:6565\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Starting emission of VUs and VUsMax metrics...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Start of initialization\" executorsCount=1 neededVUs=1 phase=execution-scheduler-init\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Initialized VU #1\" phase=execution-scheduler-init\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Finished initializing needed VUs, start initializing executors...\" phase=execution-scheduler-init\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Initialized executor default\" phase=execution-scheduler-init\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Initialization completed\" phase=execution-scheduler-init\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Start of test run\" executorsCount=1 phase=execution-scheduler-run\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"setup() is not defined or not exported, skipping!\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Start all executors...\" phase=execution-scheduler-run\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Starting executor\" executor=default startTime=0s type=shared-iterations\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Starting executor run...\" executor=shared-iterations iterations=1 maxDuration=10s scenario=default type=shared-iterations vus=1\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Regular duration is done, waiting for iterations to gracefully finish\" executor=shared-iterations gracefulStop=30s scenario=default\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Executor finished successfully\" executor=default startTime=0s type=shared-iterations\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"teardown() is not defined or not exported, skipping!\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Test finished cleanly\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Stopping vus and vux_max metrics emission...\" phase=execution-scheduler-init\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Metrics emission of VUs and VUsMax metrics stopped\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Releasing signal trap...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Waiting for metric processing to finish...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Metrics processing finished!\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Stopping outputs...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Stopping 2 outputs...\" component=output-manager\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=Stopping... component=metrics-engine-ingester\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Stopped!\" component=metrics-engine-ingester\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Generating the end-of-test summary...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Everything has finished, exiting k6 normally!\"\ntime=\"2023-06-01T13:40:26-06:00\" level=test msg=\"Non-debug message, for testing!\"\n","message":"script result"}` + "\n",
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
			expectLogs:    `{"level":"debug","metrics":"probe_data_sent_bytes{scenario=\"default\"} 0\nprobe_data_received_bytes{scenario=\"default\"} 0\nprobe_iteration_duration_seconds{scenario=\"default\"} 1.3029e-05\n","logs":"time=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Logger format: TEXT\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"k6 version: v0.43.1 ((devel), go1.20.4, linux/amd64)\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Resolving and reading test 'internal/k6runner/testdata/test.js'...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=Loading... moduleSpecifier=\"file:///home/marcelo/devel/grafana/sm-agent/internal/k6runner/testdata/test.js\" originalModuleSpecifier=internal/k6runner/testdata/test.js\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"'internal/k6runner/testdata/test.js' resolved to 'file:///home/marcelo/devel/grafana/sm-agent/internal/k6runner/testdata/test.js' and successfully loaded 31 bytes!\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Gathering k6 runtime options...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Initializing k6 runner for 'internal/k6runner/testdata/test.js' (file:///home/marcelo/devel/grafana/sm-agent/internal/k6runner/testdata/test.js)...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Detecting test type for...\" test_path=\"file:///home/marcelo/devel/grafana/sm-agent/internal/k6runner/testdata/test.js\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Trying to load as a JS test...\" test_path=\"file:///home/marcelo/devel/grafana/sm-agent/internal/k6runner/testdata/test.js\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Babel: Transformed\" t=8.993963ms\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Runner successfully initialized!\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Parsing CLI flags...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Consolidating config layers...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Parsing thresholds and validating config...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Initializing the execution scheduler...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Starting 2 outputs...\" component=output-manager\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=Starting... component=metrics-engine-ingester\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Started!\" component=metrics-engine-ingester\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Trapping interrupt signals so k6 can handle them gracefully...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Starting the REST API server on localhost:6565\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Starting emission of VUs and VUsMax metrics...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Start of initialization\" executorsCount=1 neededVUs=1 phase=execution-scheduler-init\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Initialized VU #1\" phase=execution-scheduler-init\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Finished initializing needed VUs, start initializing executors...\" phase=execution-scheduler-init\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Initialized executor default\" phase=execution-scheduler-init\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Initialization completed\" phase=execution-scheduler-init\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Start of test run\" executorsCount=1 phase=execution-scheduler-run\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"setup() is not defined or not exported, skipping!\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Start all executors...\" phase=execution-scheduler-run\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Starting executor\" executor=default startTime=0s type=shared-iterations\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Starting executor run...\" executor=shared-iterations iterations=1 maxDuration=10s scenario=default type=shared-iterations vus=1\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Regular duration is done, waiting for iterations to gracefully finish\" executor=shared-iterations gracefulStop=30s scenario=default\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Executor finished successfully\" executor=default startTime=0s type=shared-iterations\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"teardown() is not defined or not exported, skipping!\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Test finished cleanly\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Stopping vus and vux_max metrics emission...\" phase=execution-scheduler-init\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Metrics emission of VUs and VUsMax metrics stopped\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Releasing signal trap...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Waiting for metric processing to finish...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Metrics processing finished!\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Stopping outputs...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Stopping 2 outputs...\" component=output-manager\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=Stopping... component=metrics-engine-ingester\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Stopped!\" component=metrics-engine-ingester\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Generating the end-of-test summary...\"\ntime=\"2023-06-01T13:40:26-06:00\" level=debug msg=\"Everything has finished, exiting k6 normally!\"\ntime=\"2023-06-01T13:40:26-06:00\" level=test msg=\"Non-debug message, for testing!\"\n","message":"script result"}` + "\n",
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
			expectLogs:    `{"level":"error","error":"Post \"http://127.0.0.1:42731/run\": context deadline exceeded","message":"sending request"}` + "\n" +
				`{"level":"warn","error":"retryable\nmaking request: Post \"http://127.0.0.1:42731/run\": context deadline exceeded","after":0,"message":"retrying retryable error"}` + "\n" +
				`{"level":"error","error":"Post \"http://127.0.0.1:42731/run\": context deadline exceeded","message":"sending request"}` + "\n" +
				`{"level":"warn","error":"retryable\nmaking request: Post \"http://127.0.0.1:42731/run\": context deadline exceeded","after":23.035135,"message":"retrying retryable error"}` + "\n" +
				`{"level":"error","error":"retryable\nmaking request: Post \"http://127.0.0.1:42731/run\": context deadline exceeded","message":"retries exhausted"}` + "\n" +
				`{"level":"debug","error":"cannot retry further: retryable\nmaking request: Post \"http://127.0.0.1:42731/run\": context deadline exceeded\ncontext deadline exceeded","message":"k6 script exited with error code"}` + "\n",
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

			runner := HttpRunner{url: srv.URL + "/run", graceTime: time.Second, backoff: time.Second, metrics: NewHTTPMetrics(prometheus.NewRegistry())}
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
				zlogger  = zerolog.New(&logBuf)
			)

			success, err := script.Run(ctx, registry, zlogger, SecretStore{})
			require.Equal(t, tc.expectSuccess, success)
			require.Equal(t, tc.expectLogs, logBuf.String())
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

				runner := HttpRunner{url: srv.URL + "/run", graceTime: tc.graceTime, backoff: time.Second, metrics: NewHTTPMetrics(prometheus.NewRegistry())}
				processor, err := NewProcessor(Script{Script: nil, Settings: Settings{tc.scriptTimeout.Milliseconds()}}, runner)
				require.NoError(t, err)

				baseCtx, baseCancel := context.WithTimeout(context.Background(), tc.globalTimeout)
				t.Cleanup(baseCancel)
				ctx, cancel := testhelper.Context(baseCtx, t)
				t.Cleanup(cancel)

				var (
					registry = prometheus.NewRegistry()
					zlogger  = zerolog.New(io.Discard)
				)
				success, err := processor.Run(ctx, registry, zlogger, SecretStore{})
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

		listenerCh := make(chan string)

		go func() {
			listener, err := net.Listen("tcp4", "localhost:")
			require.NoError(t, err)

			listenerCh <- listener.Addr().String()

			err = http.Serve(listener, mux)
			require.NoError(t, err)

			t.Cleanup(func() {
				listener.Close()
			})
		}()

		addr := <-listenerCh

		runner := HttpRunner{url: "http://" + addr + "/run", graceTime: time.Second, backoff: time.Second, metrics: NewHTTPMetrics(prometheus.NewRegistry())}
		processor, err := NewProcessor(Script{Script: nil, Settings: Settings{Timeout: 1000}}, runner)
		require.NoError(t, err)

		baseCtx, baseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.Cleanup(baseCancel)
		ctx, cancel := testhelper.Context(baseCtx, t)
		t.Cleanup(cancel)

		var (
			registry = prometheus.NewRegistry()
			zlogger  = zerolog.New(io.Discard)
		)
		success, err := processor.Run(ctx, registry, zlogger, SecretStore{})
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
