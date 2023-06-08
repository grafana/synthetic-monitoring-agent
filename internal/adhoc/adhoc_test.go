package adhoc

import (
	"context"
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
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
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
}

func TestHandlerRun(t *testing.T) {
	features := feature.NewCollection()
	require.NoError(t, features.Set("adhoc"))

	logger := zerolog.New(io.Discard)
	if testing.Verbose() {
		logger = zerolog.New(os.Stdout)
	}

	publishCh := make(chan pusher.Payload)

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

type grpcTestConn struct {
}

func (grpcTestConn) GetState() connectivity.State {
	return connectivity.Ready
}

func (grpcTestConn) Invoke(context.Context, string, interface{}, interface{}, ...grpc.CallOption) error {
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
}

func (c *testClient) RegisterProbe(ctx context.Context, in *sm.ProbeInfo, opts ...grpc.CallOption) (*sm.RegisterProbeResult, error) {
	c.logger.Info().Str("func", "RegisterProbe").Interface("in", in).Interface("opts", opts).Send()

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

func (c *testGetAdhocChecksClient) RecvMsg(interface{}) error {
	c.logger.Info().Str("func", "RecvMsg").Caller(0).Send()
	return nil
}

func (c *testGetAdhocChecksClient) SendMsg(interface{}) error {
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

func (p *testProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	p.logger.Info().Str("func", "Probe").Caller(0).Send()
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test",
	})
	g.Set(1)
	registry.MustRegister(g)
	_ = logger.Log("msg", "test")
	return true
}
