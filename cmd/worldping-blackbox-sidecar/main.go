package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/grafana/worldping-blackbox-sidecar/internal/checks"
	"github.com/grafana/worldping-blackbox-sidecar/internal/http"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pusher"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

const exitFail = 1

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)

	var (
		debug             = flags.Bool("debug", false, "debug output (enables verbose)")
		verbose           = flags.Bool("verbose", false, "verbose logging")
		grpcApiServerAddr = flags.String("api-server-address", "localhost:4031", "GRPC API server address")
		grpcInsecure      = flags.Bool("api-insecure", false, "Don't use TLS with connections to GRPC API")
		httpListenAddr    = flags.String("listen-address", ":4050", "listen address")
		apiToken          = flags.String("api-token", "", "base64-encoded API token")
	)

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	if *apiToken == "" {
		return fmt.Errorf("invalid API token")
	}

	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, ctx := errgroup.WithContext(baseCtx)

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs

	zl := zerolog.New(stdout).With().Timestamp().Str("program", filepath.Base(args[0])).Logger()

	switch {
	case *debug:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		zl = zl.With().Caller().Logger()
		*verbose = true

	case *verbose:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)

	default:
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	}

	g.Go(func() error {
		return signalHandler(ctx, zl.With().Str("subsystem", "signal handler").Logger())
	})

	zl.Info().Str("version", version).Str("commit", commit).Str("buildstamp", buildstamp).Msg("starting")

	promRegisterer := prometheus.NewRegistry()

	if err := registerMetrics(promRegisterer); err != nil {
		return err
	}

	router := NewMux(MuxOpts{
		Logger:         zl.With().Str("subsystem", "mux").Logger(),
		PromRegisterer: promRegisterer,
	})

	httpConfig := http.Config{
		ListenAddr:   *httpListenAddr,
		Logger:       zl.With().Str("subsystem", "http").Logger(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	httpServer := http.NewServer(ctx, router, httpConfig)

	httpListener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", httpServer.ListenAddr())
	if err != nil {
		return err
	}

	g.Go(func() error {
		<-ctx.Done()
		timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer timeoutCancel()

		// we probably cannot do anything meaningful with this
		// error but return it anyways.
		return httpServer.Shutdown(timeoutCtx)
	})

	g.Go(func() error {
		return httpServer.Run(httpListener)
	})

	publishCh := make(chan pusher.Payload)

	conn, err := dialAPIServer(ctx, *grpcApiServerAddr, *grpcInsecure, *apiToken)
	if err != nil {
		return fmt.Errorf("dialing GRPC server %s: %w", *grpcApiServerAddr, err)
	}
	defer conn.Close()

	checksUpdater, err := checks.NewUpdater(conn, zl.With().Str("subsystem", "updater").Logger(), publishCh, promRegisterer)
	if err != nil {
		log.Fatalf("Cannot create checks updater: %s", err)
	}

	g.Go(func() error {
		return checksUpdater.Run(ctx)
	})

	publisher := pusher.NewPublisher(conn, publishCh, zl.With().Str("subsystem", "publisher").Logger(), promRegisterer)

	g.Go(func() error {
		return publisher.Run(ctx)
	})

	return g.Wait()
}

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "E: %s\n", err)
		os.Exit(exitFail)
	}
}

func signalHandler(ctx context.Context, logger zerolog.Logger) error {
	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info().Str("signal", sig.String()).Msg("shutting down")
		return fmt.Errorf("Got signal %s", sig)

	case <-ctx.Done():
		logger.Info().Msg("shutting down")
		return nil
	}
}
