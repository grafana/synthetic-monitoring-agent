package http

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/rs/zerolog"
)

type server struct {
	srv *http.Server
}

func NewServer(ctx context.Context, handler http.Handler, cfg Config) *server {
	stdlog := log.New(cfg.Logger, "", 0)

	wrapper := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := httpsnoop.CaptureMetrics(handler, w, r)
		var level zerolog.Level
		switch m.Code / 100 {
		case 1, 2, 3:
			level = zerolog.InfoLevel
		case 4:
			level = zerolog.WarnLevel
		case 5:
			level = zerolog.ErrorLevel
		default:
			level = zerolog.WarnLevel
		}
		cfg.Logger.WithLevel(level).
			Str("method", r.Method).
			Stringer("url", r.URL).
			Int("code", m.Code).
			Dur("duration", m.Duration).
			Msg("handled request")
	})

	return &server{
		srv: &http.Server{
			Addr:         cfg.ListenAddr,
			Handler:      wrapper,
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
