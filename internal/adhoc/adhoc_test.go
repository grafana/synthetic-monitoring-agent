package adhoc

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

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

	opts := HandlerOpts{
		Conn:           nil,
		Logger:         zerolog.New(io.Discard),
		Publisher:      channelPublisher(make(chan pusher.Payload)),
		TenantCh:       make(chan sm.Tenant),
		PromRegisterer: prometheus.NewPedanticRegistry(),
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

	logBody := payload.Streams()[0].Entries[0].Line
	require.Contains(t, logBody, "ad-hoc check done") // log must publish even when verbose is disabled for check tests
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

type grpcTestConn struct {
}

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

func (p *testProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) (bool, float64) {
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

func (r *testK6Runner) Run(ctx context.Context, script k6runner.Script, secretStore k6runner.SecretStore) (*k6runner.RunResponse, error) {
	return &k6runner.RunResponse{}, nil
}
