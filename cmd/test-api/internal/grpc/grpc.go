package grpc

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"time"

	"github.com/gogo/status"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

type ServerDb interface {
	FindProbeIDByToken(ctx context.Context, token []byte) (int64, error)
}

type Opts struct {
	Logger            zerolog.Logger
	ListenAddr        string
	ChecksServer      sm.ChecksServer
	TenantsServer     sm.TenantsServer
	AdHocChecksServer sm.AdHocChecksServer
	Db                ServerDb
}

type Server struct {
	srv    *grpc.Server
	logger zerolog.Logger
	addr   string
	db     ServerDb
}

func NewServer(ctx context.Context, opts *Opts) (*Server, error) {
	srv := &Server{
		logger: opts.Logger,
		addr:   opts.ListenAddr,
		db:     opts.Db,
	}

	srvOpts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(srv.unaryHandler),
		grpc.ChainStreamInterceptor(srv.streamHandler),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     5 * time.Minute,
			MaxConnectionAge:      time.Duration(math.MaxInt64),
			MaxConnectionAgeGrace: time.Duration(math.MaxInt64),
			Time:                  120 * time.Second,
			Timeout:               20 * time.Second,
		}),
	}

	srv.srv = grpc.NewServer(srvOpts...)

	if opts.ChecksServer != nil {
		sm.RegisterChecksServer(srv.srv, opts.ChecksServer)
	}

	if opts.TenantsServer != nil {
		sm.RegisterTenantsServer(srv.srv, opts.TenantsServer)
	}

	if opts.AdHocChecksServer != nil {
		sm.RegisterAdHocChecksServer(srv.srv, opts.AdHocChecksServer)
	}

	return srv, nil
}

func (s *Server) Run(ctx context.Context) error {
	s.logger.Info().Str("address", s.addr).Msg("starting GRPC server")

	l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.addr)
	if err != nil {
		return err
	}

	if err := s.srv.Serve(l); err != nil {
		return fmt.Errorf("could not serve GRPC on %s: %w", s.addr, err)
	}

	return nil
}

func (s *Server) GracefulStop() {
	s.srv.GracefulStop()
}

func (s *Server) Stop() {
	s.srv.Stop()
}

func (s *Server) unaryHandler(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	probeID, err := s.validateToken(ctx)
	if err != nil {
		return nil, err
	}

	return handler(contextWithProbeId(ctx, probeID), req)
}

func (s *Server) streamHandler(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	probeID, err := s.validateToken(ss.Context())
	if err != nil {
		return err
	}

	wrapper := &serverStreamWrapper{
		ctx:          contextWithProbeId(ss.Context(), probeID),
		ServerStream: ss,
	}

	return handler(srv, wrapper)
}

func (s *Server) validateToken(ctx context.Context) (int64, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return -1, errors.New("missing grpc metadata")
	}

	auth, found := md["authorization"]
	if !found {
		msg := "missing authorization token"
		err := status.Error(codes.InvalidArgument, msg)
		return -1, err
	}

	if len(auth) != 1 {
		msg := "invalid authorization metadata"
		err := status.Error(codes.InvalidArgument, msg)
		return -1, err
	}

	const prefix = "Bearer "

	if !strings.HasPrefix(auth[0], prefix) {
		msg := "invalid authorization metadata format"
		err := status.Error(codes.InvalidArgument, msg)
		return -1, err
	}

	b64token := strings.TrimPrefix(auth[0], prefix)
	if _, err := base64.StdEncoding.DecodeString(b64token); err != nil {
		msg := "invalid authorization encoding"
		err := status.Error(codes.InvalidArgument, msg)
		return -1, err
	}

	id, err := s.db.FindProbeIDByToken(ctx, []byte(b64token))
	if err != nil {
		return -1, err
	}

	return id, nil
}

type probeIdKey struct{}

func contextWithProbeId(ctx context.Context, probeId int64) context.Context {
	return context.WithValue(ctx, probeIdKey{}, probeId)
}

func probeIdFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(probeIdKey{}).(int64)
	return id, ok
}

type serverStreamWrapper struct {
	ctx context.Context
	grpc.ServerStream
}

func (w *serverStreamWrapper) Context() context.Context {
	return w.ctx
}
