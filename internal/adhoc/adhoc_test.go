package adhoc

import (
	"context"
	"errors"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/google/uuid"
	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

func TestNewHandler(t *testing.T) {
	features := feature.NewCollection()
	require.NoError(t, features.Set("adhoc"))

	registry := prometheus.NewPedanticRegistry()
	opts := HandlerOpts{
		Conn:           nil,
		Logger:         zerolog.New(io.Discard),
		Publisher:      channelPublisher(make(chan pusher.Payload)),
		TenantCh:       make(chan sm.Tenant),
		PromRegisterer: registry,
		Features:       features,
	}

	h, err := NewHandler(opts)
	require.NoError(t, err)
	require.NotNil(t, h)

	require.Equal(t, opts.Conn, h.api.conn)
	require.Equal(t, opts.Logger, h.logger)
	require.Equal(t, opts.Features, h.features)
	require.Equal(t, opts.Publisher, h.publisher)
	require.NotNil(t, h.runnerFactory)
	require.NotNil(t, h.grpcAdhocChecksClientFactory)
	require.Nil(t, h.probe, "probe should not be set at this point")
	require.NotNil(t, h.metrics.opsCounter)
	require.NotNil(t, h.metrics.k6GraceTime)
	require.Equal(t, k6AdhocGraceTime.Seconds(), testutil.ToFloat64(h.metrics.k6GraceTime))

	metricCount, err := testutil.GatherAndCount(registry, "sm_adhoc_k6_grace_time_seconds")
	require.NoError(t, err)
	require.Equal(t, 1, metricCount)
	require.False(t, h.supportsProtocolSecrets, "default value should be false")
}

func TestHandlerSupportsProtocolSecrets(t *testing.T) {
	features := feature.NewCollection()
	require.NoError(t, features.Set("adhoc"))

	var capturedProbeInfo *sm.ProbeInfo

	testClient := &testClient{
		logger: zerolog.New(io.Discard),
		registerProbeHook: func(info *sm.ProbeInfo) {
			capturedProbeInfo = info
		},
	}

	opts := HandlerOpts{
		Conn:                    &grpcTestConn{},
		Logger:                  zerolog.New(io.Discard),
		Publisher:               channelPublisher(make(chan pusher.Payload)),
		TenantCh:                make(chan sm.Tenant),
		PromRegisterer:          prometheus.NewPedanticRegistry(),
		Features:                features,
		SupportsProtocolSecrets: true,
		grpcAdhocChecksClientFactory: func(conn ClientConn) (sm.AdHocChecksClient, error) {
			return testClient, nil
		},
	}

	h, err := NewHandler(opts)
	require.NoError(t, err)
	require.NotNil(t, h)
	require.True(t, h.supportsProtocolSecrets, "should be set to true")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Run will call RegisterProbe, which should capture the ProbeInfo
	_ = h.Run(ctx)

	require.NotNil(t, capturedProbeInfo, "RegisterProbe should have been called")
	require.True(t, capturedProbeInfo.SupportsProtocolSecrets, "SupportsProtocolSecrets should be true")
}

func TestHandlerIsClusterEnabled(t *testing.T) {
	features := feature.NewCollection()
	require.NoError(t, features.Set("adhoc"))

	var capturedProbeInfo *sm.ProbeInfo

	testClient := &testClient{
		logger: zerolog.New(io.Discard),
		registerProbeHook: func(info *sm.ProbeInfo) {
			capturedProbeInfo = info
		},
	}

	opts := HandlerOpts{
		Conn:             &grpcTestConn{},
		Logger:           zerolog.New(io.Discard),
		Publisher:        channelPublisher(make(chan pusher.Payload)),
		TenantCh:         make(chan sm.Tenant),
		PromRegisterer:   prometheus.NewPedanticRegistry(),
		Features:         features,
		IsClusterEnabled: true,
		grpcAdhocChecksClientFactory: func(conn ClientConn) (sm.AdHocChecksClient, error) {
			return testClient, nil
		},
	}

	h, err := NewHandler(opts)
	require.NoError(t, err)
	require.NotNil(t, h)
	require.True(t, h.isClusterEnabled, "should be set to true")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Run will call RegisterProbe, which should capture the ProbeInfo
	_ = h.Run(ctx)

	require.NotNil(t, capturedProbeInfo, "RegisterProbe should have been called")
	require.True(t, capturedProbeInfo.IsClusterEnabled, "IsClusterEnabled should be true")
}

func TestHandlerRun(t *testing.T) {
	features := feature.NewCollection()
	require.NoError(t, features.Set("adhoc"))

	logger := zerolog.New(io.Discard)
	if testing.Verbose() {
		logger = zerolog.New(os.Stdout)
	}

	publishCh := make(chan pusher.Payload)

	zerolog.SetGlobalLevel(zerolog.WarnLevel) // default log level.

	opts := HandlerOpts{
		Logger:         logger,
		Publisher:      channelPublisher(publishCh),
		TenantCh:       make(chan sm.Tenant),
		PromRegisterer: prometheus.NewPedanticRegistry(),
		Features:       features,
		runnerFactory: func(ctx context.Context, req *sm.AdHocRequest) (*runner, error) {
			return &runner{
				logger: logger,
				prober: &testProber{logger},
				id:     req.AdHocCheck.Id,
				target: req.AdHocCheck.Target,
				probe:  "testProbe",
			}, nil
		},
		grpcAdhocChecksClientFactory: func(conn ClientConn) (sm.AdHocChecksClient, error) {
			return &testClient{logger: logger}, nil
		},
	}

	h, err := NewHandler(opts)
	require.NoError(t, err)
	require.NotNil(t, h)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	runErr := h.Run(ctx)
	require.NoError(t, runErr)

	payload := <-publishCh
	require.Len(t, payload.Metrics(), 0)
	require.Len(t, payload.Streams(), 1)

	entry := payload.Streams()[0].Entries[0]
	require.Contains(t, entry.Line, "ad-hoc check done") // log must publish even when verbose is disabled for check tests
	require.Len(t, entry.StructuredMetadata, 1)
	require.Equal(t, "execution_id", entry.StructuredMetadata[0].Name)
	_, err = uuid.Parse(entry.StructuredMetadata[0].Value)
	require.NoError(t, err, "execution_id should be a valid UUID")
}

func TestHandleAdHocCheckDoesNotLogCredentials(t *testing.T) {
	const (
		checkPassword  = "randompasswordinplaintext"
		remotePassword = "theremotepassword"
	)

	features := feature.NewCollection()
	require.NoError(t, features.Set("adhoc"))

	logs := &testhelper.LogBuffer{}

	globalLevel := zerolog.GlobalLevel()

	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	t.Cleanup(func() { zerolog.SetGlobalLevel(globalLevel) })

	logger := zerolog.New(logs).Level(zerolog.DebugLevel)

	h, err := NewHandler(HandlerOpts{
		Logger:         logger,
		Publisher:      channelPublisher(make(chan pusher.Payload, 1)),
		TenantCh:       make(chan sm.Tenant, 1),
		PromRegisterer: prometheus.NewPedanticRegistry(),
		Features:       features,
		runnerFactory: func(ctx context.Context, req *sm.AdHocRequest) (*runner, error) {
			return &runner{
				logger: logger,
				prober: &testProber{logger},
				id:     req.AdHocCheck.Id,
				target: req.AdHocCheck.Target,
				probe:  "testProbe",
			}, nil
		},
	})
	require.NoError(t, err)

	request := sm.AdHocRequest{
		AdHocCheck: sm.AdHocCheck{
			Id:       "8f8b7b4e-1f5d-4b0f-9b3a-000000000000",
			TenantId: 1,
			Timeout:  10000,
			Target:   "https://intranet.example.org/alerts",
			Settings: sm.CheckSettings{
				Http: &sm.HttpSettings{
					BasicAuth: &sm.BasicAuth{
						Username: "grafana",
						Password: checkPassword,
					},
				},
			},
		},
		Tenant: &sm.Tenant{
			Id: 1,
			MetricsRemote: &sm.RemoteInfo{
				Name:     "metrics",
				Url:      "https://example.org/push",
				Username: "1",
				Password: remotePassword,
			},
		},
	}

	require.NoError(t, h.handleAdHocCheck(t.Context(), &request))

	actual := logs.String()

	require.Contains(t, actual, "got ad-hoc check request", "the log line under test did not make it to the logger")
	require.NotContains(t, actual, checkPassword, "the debug log must not leak the HTTP basic auth password")
	require.NotContains(t, actual, remotePassword, "the debug log must not leak the tenant remote write password")
	require.Contains(t, actual, request.AdHocCheck.Id)
}

type channelPublisher chan pusher.Payload

func (c channelPublisher) Publish(payload pusher.Payload) {
	c <- payload
}

type grpcTestError struct {
	code codes.Code
	msg  string
}

func newGrpcTestError(code codes.Code, msg string) *grpcTestError {
	return &grpcTestError{
		code: code,
		msg:  msg,
	}
}

func (e grpcTestError) Error() string {
	return e.msg
}

func (e grpcTestError) GRPCStatus() *status.Status {
	return status.New(e.code, e.msg)
}

type testBackoff struct{}

func (testBackoff) Duration() time.Duration {
	return 0
}

func (testBackoff) Reset() {}

type timeoutBackoff struct{}

func (timeoutBackoff) Duration() time.Duration {
	return 100 * time.Millisecond
}

func (timeoutBackoff) Reset() {}

type grpcTestConn struct{}

func (grpcTestConn) GetState() connectivity.State {
	return connectivity.Ready
}

func (grpcTestConn) Invoke(context.Context, string, any, any, ...grpc.CallOption) error {
	return nil
}

func (grpcTestConn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, nil
}

func TestHandlerRunErrors(t *testing.T) {
	features := feature.NewCollection()
	require.NoError(t, features.Set("adhoc"))

	logger := zerolog.New(io.Discard)
	if testing.Verbose() {
		logger = zerolog.New(os.Stdout)
	}

	publishCh := make(chan pusher.Payload)

	testcases := map[string]struct {
		expectError         bool
		registerProbeError  error
		getAdhocChecksError error
	}{
		"canceled": {
			expectError:        false,
			registerProbeError: newGrpcTestError(codes.Canceled, "context canceled"),
		},
		"unimplemented": {
			expectError:        true,
			registerProbeError: newGrpcTestError(codes.Unimplemented, "not implemented"),
		},
		"permission denied": {
			expectError:        true,
			registerProbeError: newGrpcTestError(codes.PermissionDenied, "not authorized"),
		},
		"idle timeout": {
			expectError:        true,
			registerProbeError: newGrpcTestError(codes.Unavailable, "unexpected HTTP status code received from server: 504 (Gateway Timeout); transport: received unexpected content-type \"text/html\""),
		},
		// "transport closing": {
		// 	registerProbeError: newGrpcTestError(codes.Aborted, "transport is closing"),
		// },
		// "other": {
		// 	registerProbeError: errors.New("arbitrary error"),
		// },
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			tc := tc

			// Use timeoutBackoff for idle timeout test to prevent infinite loop
			var backoff Backoffer = testBackoff{}
			if name == "idle timeout" {
				backoff = timeoutBackoff{}
			}

			opts := HandlerOpts{
				Conn:           &grpcTestConn{},
				Logger:         logger,
				Publisher:      channelPublisher(publishCh),
				Backoff:        backoff,
				TenantCh:       make(chan sm.Tenant),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				Features:       features,
				runnerFactory: func(ctx context.Context, req *sm.AdHocRequest) (*runner, error) {
					return &runner{
						logger: logger,
						prober: &testProber{logger},
						id:     req.AdHocCheck.Id,
						target: req.AdHocCheck.Target,
						probe:  "testProbe",
					}, nil
				},
				grpcAdhocChecksClientFactory: func(conn ClientConn) (sm.AdHocChecksClient, error) {
					return &testClient{logger: logger, registerProbeError: tc.registerProbeError}, nil
				},
			}

			h, err := NewHandler(opts)
			require.NoError(t, err)
			require.NotNil(t, h)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			runErr := h.Run(ctx)
			if tc.expectError {
				require.Error(t, runErr)
			} else {
				require.NoError(t, runErr)
			}

			select {
			case <-publishCh:
				t.Fail()
			default:
			}
		})
	}
}

type testClient struct {
	logger              zerolog.Logger
	registerProbeError  error
	getAdhocChecksError error
	registerProbeHook   func(*sm.ProbeInfo)

	// results, if set, records the results passed to ReportAdHocResult.
	// If nil, ReportAdHocResult behaves like an API that predates the
	// RPC.
	results *resultRecorder
	// reportErr, if set, is returned by ReportAdHocResult.
	reportErr error
	// reportCalls counts the calls to ReportAdHocResult.
	reportCalls atomic.Int32
	// reportBounded is set if a call had a deadline of at most
	// reportTimeout.
	reportBounded atomic.Bool
}

func (c *testClient) RegisterProbe(ctx context.Context, in *sm.ProbeInfo, opts ...grpc.CallOption) (*sm.RegisterProbeResult, error) {
	c.logger.Info().Str("func", "RegisterProbe").Interface("in", in).Interface("opts", opts).Send()

	if c.registerProbeHook != nil {
		c.registerProbeHook(in)
	}

	if c.registerProbeError != nil {
		return nil, c.registerProbeError
	}

	return &sm.RegisterProbeResult{
		Probe: sm.Probe{
			Id:         1,
			TenantId:   1000,
			Name:       "testProbe",
			Version:    in.Version,
			Commit:     in.Commit,
			Buildstamp: in.Buildstamp,
		},
		Status: sm.Status{
			Code:    sm.StatusCode_OK,
			Message: "OK",
		},
	}, nil
}

func (c *testClient) ReportAdHocResult(ctx context.Context, in *sm.AdHocResult, opts ...grpc.CallOption) (*sm.Void, error) {
	c.logger.Info().Str("func", "ReportAdHocResult").Interface("in", in).Interface("opts", opts).Send()
	c.reportCalls.Add(1)

	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= reportTimeout {
		c.reportBounded.Store(true)
	}

	switch {
	case c.reportErr != nil:
		return nil, c.reportErr

	case c.results != nil:
		c.results.ch <- in

		return &sm.Void{}, nil

	default:
		// Mimic an API that predates the RPC.
		return nil, status.Error(codes.Unimplemented, "ReportAdHocResult not implemented")
	}
}

// resultRecorder records the results reported to the API.
type resultRecorder struct {
	ch chan *sm.AdHocResult
}

func newResultRecorder() *resultRecorder {
	return &resultRecorder{ch: make(chan *sm.AdHocResult, 16)}
}

// wait blocks until a result is reported and returns it.
func (r *resultRecorder) wait(t *testing.T) *sm.AdHocResult {
	t.Helper()

	select {
	case result := <-r.ch:
		return result

	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a result to be reported")

		return nil
	}
}

// blockingProber blocks until its context is done, like a check that
// hits its timeout.
type blockingProber struct{}

func (blockingProber) Name() string { return "blocking" }

func (blockingProber) Probe(ctx context.Context, _ string, _ *prometheus.Registry, _ logger.Logger, _ string) (bool, float64) {
	<-ctx.Done()

	return false, 0
}

// newReportTestHandler returns a Handler wired to client, and the
// channel where published payloads end up. If factory is nil the runner
// uses testProber.
func newReportTestHandler(t *testing.T, client *testClient, factory func(context.Context, *sm.AdHocRequest) (*runner, error)) (*Handler, chan pusher.Payload) {
	t.Helper()

	features := feature.NewCollection()
	require.NoError(t, features.Set("adhoc"))

	logger := zerolog.New(io.Discard)
	client.logger = logger

	if factory == nil {
		factory = func(_ context.Context, req *sm.AdHocRequest) (*runner, error) {
			return &runner{
				logger: logger,
				prober: &testProber{logger},
				id:     req.AdHocCheck.Id,
				target: req.AdHocCheck.Target,
				probe:  "testProbe",
			}, nil
		}
	}

	publishCh := make(chan pusher.Payload, 1)

	h, err := NewHandler(HandlerOpts{
		Logger:         logger,
		Publisher:      channelPublisher(publishCh),
		TenantCh:       make(chan sm.Tenant),
		PromRegisterer: prometheus.NewPedanticRegistry(),
		Features:       features,
		runnerFactory:  factory,
		grpcAdhocChecksClientFactory: func(ClientConn) (sm.AdHocChecksClient, error) {
			return client, nil
		},
	})
	require.NoError(t, err)

	return h, publishCh
}

func requirePublished(t *testing.T, publishCh <-chan pusher.Payload) {
	t.Helper()

	select {
	case payload := <-publishCh:
		require.Len(t, payload.Streams(), 1)
		require.Contains(t, payload.Streams()[0].Entries[0].Line, "ad-hoc check done")

	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the Loki publish")
	}
}

// The "id" used by testGetAdhocChecksClient.Recv.
const testAdHocID = "test"

func TestReportAdHocResultSuccess(t *testing.T) {
	rec := newResultRecorder()
	h, publishCh := newReportTestHandler(t, &testClient{results: rec}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, h.Run(ctx))

	result := rec.wait(t)
	requirePublished(t, publishCh)

	require.Equal(t, testAdHocID, result.ExecutionId)
	require.Equal(t, int64(1), result.ProbeId, "probe id comes from RegisterProbe")
	require.Equal(t, sm.AdHocResult_OK, result.Status)
	require.Equal(t, float64(1), result.DurationSeconds)
	require.Empty(t, result.Error)
}

func TestReportAdHocResultCouldNotStart(t *testing.T) {
	rec := newResultRecorder()
	h, publishCh := newReportTestHandler(t, &testClient{results: rec},
		func(context.Context, *sm.AdHocRequest) (*runner, error) {
			return nil, errors.New("boom")
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, h.Run(ctx))

	result := rec.wait(t)

	require.Equal(t, testAdHocID, result.ExecutionId)
	require.Equal(t, sm.AdHocResult_COULD_NOT_START, result.Status)
	require.Contains(t, result.Error, "boom")

	select {
	case <-publishCh:
		t.Fatal("nothing ran, nothing should be published")
	default:
	}
}

func TestReportAdHocResultTimeout(t *testing.T) {
	rec := newResultRecorder()
	h, publishCh := newReportTestHandler(t, &testClient{results: rec},
		func(_ context.Context, req *sm.AdHocRequest) (*runner, error) {
			return &runner{
				logger:  zerolog.New(io.Discard),
				prober:  blockingProber{},
				id:      req.AdHocCheck.Id,
				target:  req.AdHocCheck.Target,
				probe:   "testProbe",
				timeout: 50 * time.Millisecond,
			}, nil
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, h.Run(ctx))

	result := rec.wait(t)
	require.Equal(t, sm.AdHocResult_TIMEOUT, result.Status)

	requirePublished(t, publishCh)
}

// A probe that ran the check but did not succeed reports FAILED, which is
// different from a timeout or a check that could not start.
func TestReportAdHocResultFailed(t *testing.T) {
	rec := newResultRecorder()
	h, publishCh := newReportTestHandler(t, &testClient{results: rec},
		func(_ context.Context, req *sm.AdHocRequest) (*runner, error) {
			return &runner{
				logger:  zerolog.New(io.Discard),
				prober:  failingProber{},
				id:      req.AdHocCheck.Id,
				target:  req.AdHocCheck.Target,
				probe:   "testProbe",
				timeout: 10 * time.Second,
			}, nil
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, h.Run(ctx))

	result := rec.wait(t)
	require.Equal(t, sm.AdHocResult_FAILED, result.Status)

	requirePublished(t, publishCh)
}

// failingProber runs to completion and reports that the check failed.
type failingProber struct{}

func (failingProber) Name() string { return "failing" }

func (failingProber) Probe(context.Context, string, *prometheus.Registry, logger.Logger, string) (bool, float64) {
	return false, 0.5
}

// An API that predates ReportAdHocResult answers Unimplemented. The
// probe must carry on, publishing to Loki as it always did.
func TestReportAdHocResultUnimplementedFallsBackToLoki(t *testing.T) {
	client := &testClient{} // no results recorder: Unimplemented.
	h, publishCh := newReportTestHandler(t, client, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, h.Run(ctx))

	requirePublished(t, publishCh)
	require.LessOrEqual(t, client.reportCalls.Load(), int32(1), "one attempt per execution, no retry storm")
}

// Reporting is additive: the Loki path must be unchanged whether or not
// reporting works.
func TestReportAdHocResultKeepsLokiPath(t *testing.T) {
	for name, client := range map[string]*testClient{
		"reporting works":   {results: newResultRecorder()},
		"reporting fails":   {reportErr: status.Error(codes.Internal, "kaboom")},
		"not implemented":   {},
		"unavailable":       {reportErr: status.Error(codes.Unavailable, "gone")},
		"permission denied": {reportErr: status.Error(codes.PermissionDenied, "no")},
	} {
		t.Run(name, func(t *testing.T) {
			h, publishCh := newReportTestHandler(t, client, nil)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			require.NoError(t, h.Run(ctx))

			select {
			case payload := <-publishCh:
				require.Len(t, payload.Metrics(), 0)
				require.Len(t, payload.Streams(), 1)

				entry := payload.Streams()[0].Entries[0]
				require.Contains(t, entry.Line, "ad-hoc check done")
				require.Len(t, entry.StructuredMetadata, 1)
				require.Equal(t, "execution_id", entry.StructuredMetadata[0].Name)

				_, err := uuid.Parse(entry.StructuredMetadata[0].Value)
				require.NoError(t, err, "execution_id should be a valid UUID")

			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for the Loki publish")
			}
		})
	}
}

// A reporting call that never returns must not hold the goroutine forever,
// so the call is bounded by a deadline.
func TestReportAdHocResultCallIsBounded(t *testing.T) {
	client := &testClient{results: newResultRecorder(), logger: zerolog.New(io.Discard)}

	// The context has no deadline of its own, so any deadline the call
	// has was added by reportResult.
	reportResult(context.Background(), client, &sm.AdHocResult{ExecutionId: testAdHocID}, zerolog.New(io.Discard))

	client.results.wait(t)

	require.True(t, client.reportBounded.Load(), "the call must have a deadline of at most reportTimeout")
}

type testGetAdhocChecksClient struct {
	count  int
	logger zerolog.Logger
	ctx    context.Context
}

func (c *testClient) GetAdHocChecks(ctx context.Context, in *sm.Void, opts ...grpc.CallOption) (sm.AdHocChecks_GetAdHocChecksClient, error) {
	c.logger.Info().Str("func", "GetAdHocChecks").Interface("in", in).Interface("opts", opts).Send()

	if c.getAdhocChecksError != nil {
		return nil, c.getAdhocChecksError
	}

	return &testGetAdhocChecksClient{logger: c.logger, ctx: ctx}, nil
}

func (c *testGetAdhocChecksClient) Recv() (*sm.AdHocRequest, error) {
	c.logger.Info().Str("func", "Recv").Caller(0).Send()
	c.count++

	switch c.count {
	case 1:
		return &sm.AdHocRequest{
			AdHocCheck: sm.AdHocCheck{
				Id:      "test",
				Target:  "testTarget",
				Timeout: 1,
				Settings: sm.CheckSettings{
					Ping: &sm.PingSettings{},
				},
			},
		}, nil

	default:
		return nil, io.EOF
	}
}

func (c *testGetAdhocChecksClient) CloseSend() error {
	c.logger.Info().Str("func", "CloseSend").Caller(0).Send()
	return nil
}

func (c *testGetAdhocChecksClient) Context() context.Context {
	c.logger.Info().Str("func", "Context").Caller(0).Send()
	return c.ctx
}

func (c *testGetAdhocChecksClient) Header() (metadata.MD, error) {
	c.logger.Info().Str("func", "Header").Caller(0).Send()
	return nil, nil
}

func (c *testGetAdhocChecksClient) Trailer() metadata.MD {
	c.logger.Info().Str("func", "Trailer").Caller(0).Send()
	return nil
}

func (c *testGetAdhocChecksClient) RecvMsg(any) error {
	c.logger.Info().Str("func", "RecvMsg").Caller(0).Send()
	return nil
}

func (c *testGetAdhocChecksClient) SendMsg(any) error {
	c.logger.Info().Str("func", "SendMsg").Caller(0).Send()
	return nil
}

type testProber struct {
	logger zerolog.Logger
}

func (p *testProber) Name() string {
	p.logger.Info().Str("func", "Name").Caller(0).Send()
	return "test"
}

func (p *testProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger, _ string) (bool, float64) {
	p.logger.Info().Str("func", "Probe").Caller(0).Send()

	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test",
	})
	g.Set(1)
	registry.MustRegister(g)

	_ = logger.Log("msg", "test")

	return true, 1
}

func TestIdleTimeoutHandling(t *testing.T) {
	features := feature.NewCollection()
	require.NoError(t, features.Set("adhoc"))

	logger := zerolog.New(io.Discard)
	if testing.Verbose() {
		logger = zerolog.New(os.Stdout)
	}

	publishCh := make(chan pusher.Payload)

	opts := HandlerOpts{
		Conn:           &grpcTestConn{},
		Logger:         logger,
		Publisher:      channelPublisher(publishCh),
		Backoff:        testBackoff{},
		TenantCh:       make(chan sm.Tenant),
		PromRegisterer: prometheus.NewPedanticRegistry(),
		Features:       features,
		runnerFactory: func(ctx context.Context, req *sm.AdHocRequest) (*runner, error) {
			return &runner{
				logger: logger,
				prober: &testProber{logger},
				id:     req.AdHocCheck.Id,
				target: req.AdHocCheck.Target,
				probe:  "testProbe",
			}, nil
		},
		grpcAdhocChecksClientFactory: func(conn ClientConn) (sm.AdHocChecksClient, error) {
			return &testClient{
				logger:             logger,
				registerProbeError: newGrpcTestError(codes.Unavailable, "unexpected HTTP status code received from server: 504 (Gateway Timeout); transport: received unexpected content-type \"text/html\""),
			}, nil
		},
	}

	h, err := NewHandler(opts)
	require.NoError(t, err)
	require.NotNil(t, h)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	runErr := h.Run(ctx)
	require.Error(t, runErr)
	require.ErrorIs(t, runErr, context.DeadlineExceeded)

	// Should not publish anything for idle timeout
	select {
	case <-publishCh:
		t.Fail()
	default:
	}
}

func TestDefaultRunnerFactory(t *testing.T) {
	t.Parallel()

	features := feature.NewCollection()
	require.NoError(t, features.Set(feature.K6))

	logger := zerolog.New(io.Discard)
	if testing.Verbose() {
		logger = zerolog.New(os.Stdout)
	}

	// Initialize the mockRunner and secretStore
	mockRunner := &testK6Runner{}
	secretStore := &testhelper.TestSecretStore{}

	testcases := map[string]struct {
		request     *sm.AdHocRequest
		expectError bool
		errCheck    func(error) bool
		shouldPanic bool
	}{
		"ping check": {
			request: &sm.AdHocRequest{
				AdHocCheck: sm.AdHocCheck{
					Id:       "test-ping",
					TenantId: 1000,
					Target:   "example.com",
					Timeout:  1000,
					Settings: sm.CheckSettings{
						Ping: &sm.PingSettings{},
					},
				},
			},
		},
		"http check": {
			request: &sm.AdHocRequest{
				AdHocCheck: sm.AdHocCheck{
					Id:       "test-http",
					TenantId: 1000,
					Target:   "http://example.com",
					Timeout:  1000,
					Settings: sm.CheckSettings{
						Http: &sm.HttpSettings{},
					},
				},
			},
		},
		"dns check": {
			request: &sm.AdHocRequest{
				AdHocCheck: sm.AdHocCheck{
					Id:       "test-dns",
					TenantId: 1000,
					Target:   "example.com",
					Timeout:  1000,
					Settings: sm.CheckSettings{
						Dns: &sm.DnsSettings{},
					},
				},
			},
		},
		"tcp check": {
			request: &sm.AdHocRequest{
				AdHocCheck: sm.AdHocCheck{
					Id:       "test-tcp",
					TenantId: 1000,
					Target:   "example.com:80",
					Timeout:  1000,
					Settings: sm.CheckSettings{
						Tcp: &sm.TcpSettings{},
					},
				},
			},
		},
		"k6 scripted check": {
			request: &sm.AdHocRequest{
				AdHocCheck: sm.AdHocCheck{
					Id:       "test-scripted",
					TenantId: 1000,
					Target:   "test-target",
					Timeout:  1000,
					Settings: sm.CheckSettings{
						Scripted: &sm.ScriptedSettings{},
					},
				},
			},
		},
		"k6 multihttp check": {
			request: &sm.AdHocRequest{
				AdHocCheck: sm.AdHocCheck{
					Id:       "test-multihttp",
					TenantId: 1000,
					Target:   "test-target",
					Timeout:  1000,
					Settings: sm.CheckSettings{
						Multihttp: &sm.MultiHttpSettings{
							Entries: []*sm.MultiHttpEntry{
								{
									Request: &sm.MultiHttpEntryRequest{
										Url: "http://example.com",
									},
								},
							},
						},
					},
				},
			},
		},
		"k6 browser check": {
			request: &sm.AdHocRequest{
				AdHocCheck: sm.AdHocCheck{
					Id:       "test-browser",
					TenantId: 1000,
					Target:   "test-target",
					Timeout:  1000,
					Settings: sm.CheckSettings{
						Browser: &sm.BrowserSettings{},
					},
				},
			},
		},
		"zero timeout": {
			request: &sm.AdHocRequest{
				AdHocCheck: sm.AdHocCheck{
					Id:       "test-zero-timeout",
					TenantId: 1000,
					Target:   "example.com",
					Timeout:  0,
					Settings: sm.CheckSettings{
						Ping: &sm.PingSettings{},
					},
				},
			},
		},
		"empty settings": {
			request: &sm.AdHocRequest{
				AdHocCheck: sm.AdHocCheck{
					Id:       "test-empty-settings",
					TenantId: 1000,
					Target:   "example.com",
					Timeout:  1000,
					Settings: sm.CheckSettings{},
				},
			},
			expectError: true,
			shouldPanic: true,
		},
		"nil request": {
			request:     nil,
			expectError: true,
			errCheck: func(err error) bool {
				return errors.Is(err, errInvalidAdHocRequest)
			},
		},
		"zero tenant": {
			request: &sm.AdHocRequest{
				AdHocCheck: sm.AdHocCheck{
					Id:       "test-zero-tenant",
					TenantId: 0,
					Target:   "example.com",
					Timeout:  1000,
					Settings: sm.CheckSettings{},
				},
			},
			expectError: true,
			errCheck: func(err error) bool {
				return errors.Is(err, errInvalidAdHocRequest)
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := &Handler{
				logger:   logger,
				features: features,
				probe: &sm.Probe{
					Name: "test-probe",
				},
				proberFactory: prober.NewProberFactory(mockRunner, 0, features, secretStore),
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if tc.shouldPanic {
				require.Panics(t, func() {
					_, _ = h.defaultRunnerFactory(ctx, tc.request)
				})

				return
			}

			runner, err := h.defaultRunnerFactory(ctx, tc.request)
			if tc.expectError {
				require.Error(t, err)

				if tc.errCheck != nil {
					require.True(t, tc.errCheck(err), "unexpected error: %v", err)
				}

				require.Nil(t, runner)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, runner)

			// Verify runner fields
			require.Equal(t, logger, runner.logger)
			require.NotNil(t, runner.prober)
			require.Equal(t, tc.request.AdHocCheck.Id, runner.id)
			require.Equal(t, "test-probe", runner.probe)

			// For k6-based checks, verify the grace period is added
			switch tc.request.AdHocCheck.Type() {
			case sm.CheckTypeMultiHttp, sm.CheckTypeScripted, sm.CheckTypeBrowser:
				expectedTimeout := time.Duration(tc.request.AdHocCheck.Timeout)*time.Millisecond + k6AdhocGraceTime
				require.Equal(t, expectedTimeout, runner.timeout)

			default:
				expectedTimeout := time.Duration(tc.request.AdHocCheck.Timeout) * time.Millisecond
				require.Equal(t, expectedTimeout, runner.timeout)
			}
		})
	}
}

// Add mock k6runner
type testK6Runner struct{}

func (r *testK6Runner) WithLogger(logger *zerolog.Logger) k6runner.Runner {
	return r
}

func (r *testK6Runner) Run(ctx context.Context, script k6runner.Script, secretStore k6runner.SecretStore, _ string) (*k6runner.RunResponse, error) {
	return &k6runner.RunResponse{}, nil
}

func (*testK6Runner) Versions(_ context.Context) <-chan []string {
	return nil // Blocks forever if read.
}

// Errors the API returns for reasons that are expected and not worth
// retrying, like an execution it does not track (every plain "test" run),
// must not show up as warnings.
func TestReportResultLogLevels(t *testing.T) {
	for name, tc := range map[string]struct {
		code codes.Code
		warn bool
	}{
		"unimplemented":       {codes.Unimplemented, false},
		"not found":           {codes.NotFound, false},
		"already exists":      {codes.AlreadyExists, false},
		"failed precondition": {codes.FailedPrecondition, false},
		"permission denied":   {codes.PermissionDenied, true},
		"invalid argument":    {codes.InvalidArgument, true},
		"internal":            {codes.Internal, true},
		"unavailable":         {codes.Unavailable, true},
	} {
		t.Run(name, func(t *testing.T) {
			logs := &testhelper.LogBuffer{}
			logger := zerolog.New(logs).Level(zerolog.DebugLevel)

			client := &testClient{
				logger:    zerolog.New(io.Discard),
				reportErr: status.Error(tc.code, "nope"),
			}

			reportResult(context.Background(), client, &sm.AdHocResult{ExecutionId: testAdHocID}, logger)

			require.Equal(t, int32(1), client.reportCalls.Load(), "one attempt, no retries")

			if tc.warn {
				require.Contains(t, logs.String(), `"level":"warn"`)
			} else {
				require.NotContains(t, logs.String(), `"level":"warn"`)
			}
		})
	}
}
