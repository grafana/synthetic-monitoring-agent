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

	"github.com/grafana/synthetic-monitoring-agent/internal/checks"
	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/http"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

const exitFail = 1

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet(filepath.Base(args[0]), flag.ExitOnError)

	var (
		features          = feature.NewCollection()
		debug             = flags.Bool("debug", false, "debug output (enables verbose)")
		verbose           = flags.Bool("verbose", false, "verbose logging")
		reportVersion     = flags.Bool("version", false, "report version and exit")
		grpcApiServerAddr = flags.String("api-server-address", "localhost:4031", "GRPC API server address")
		grpcInsecure      = flags.Bool("api-insecure", false, "Don't use TLS with connections to GRPC API")
		httpListenAddr    = flags.String("listen-address", ":4050", "listen address")
		apiToken          = flags.String("api-token", "", "synthetic monitoring probe authentication token")
	)

	flags.Var(&features, "features", "optional feature flags")

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	if *reportVersion {
		fmt.Printf(
			"%s version=\"%s\" buildstamp=\"%s\" commit=\"%s\"\n",
			flags.Name(),
			version.Short(),
			version.Buildstamp(),
			version.Commit(),
		)
		return nil
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

	zl.Info().
		Str("version", version.Short()).
		Str("commit", version.Commit()).
		Str("buildstamp", version.Buildstamp()).
		Str("features", features.String()).
		Msg("starting")

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
		WriteTimeout: 60 * time.Second,
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
	tenantCh := make(chan synthetic_monitoring.Tenant)

	conn, err := dialAPIServer(ctx, *grpcApiServerAddr, *grpcInsecure, *apiToken)
	if err != nil {
		return fmt.Errorf("dialing GRPC server %s: %w", *grpcApiServerAddr, err)
	}
	defer conn.Close()

	checksUpdater, err := checks.NewUpdater(checks.UpdaterOptions{
		Conn:           conn,
		Logger:         zl.With().Str("subsystem", "updater").Logger(),
		PublishCh:      publishCh,
		TenantCh:       tenantCh,
		PromRegisterer: promRegisterer,
		Features:       features,
	})
	if err != nil {
		log.Fatalf("Cannot create checks updater: %s", err)
	}

	g.Go(func() error {
		return checksUpdater.Run(ctx)
	})

	tm := pusher.NewTenantManager(ctx, synthetic_monitoring.NewTenantsClient(conn), tenantCh, 15*time.Minute)

	publisher := pusher.NewPublisher(tm, publishCh, zl.With().Str("subsystem", "publisher").Logger(), promRegisterer)

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
