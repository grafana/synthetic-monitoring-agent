package http

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

type server struct {
	srv *http.Server
}

func NewServer(ctx context.Context, handler http.Handler, cfg Config) *server {
	stdlog := log.New(cfg.Logger, "", 0)

	return &server{
		srv: &http.Server{
			Addr:         cfg.ListenAddr,
			Handler:      handler,
			ErrorLog:     stdlog,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
			BaseContext:  func(net.Listener) context.Context { return ctx },
		},
	}
}

func (s *server) ListenAddr() string {
	return s.srv.Addr
}

func (s *server) Run(l net.Listener) error {
	s.srv.ErrorLog.Printf("Starting HTTP server on %s...", s.srv.Addr)

	err := s.srv.Serve(l)
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("could not serve HTTP on %s: %w", s.srv.Addr, err)
	}

	return nil
}

func (s *server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

type Config struct {
	ListenAddr   string
	Logger       zerolog.Logger
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}
