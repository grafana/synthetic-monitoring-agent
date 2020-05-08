package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/grafana/worldping-blackbox-sidecar/internal/checks"
	"github.com/grafana/worldping-blackbox-sidecar/internal/http"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pusher"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
)

const exitFail = 1

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)

	var (
		verbose             = flags.Bool("verbose", false, "verbose logging")
		bbeConfigFilename   = flags.String("blackbox-exporter-config", "worldping.yaml", "filename for blackbox exporter configuration")
		blackboxExporterStr = flags.String("blackbox-exporter-url", "http://localhost:9115/", "base URL for blackbox exporter")
		grpcApiServerAddr   = flags.String("api-server-address", "localhost:4031", "GRPC API server address")
		grpcInsecure        = flags.Bool("api-insecure", false, "Don't use TLS with connections to GRPC API")
		httpListenAddr      = flags.String("listen-address", ":4050", "listen address")
		apiToken            = flags.String("api-token", "", "base64-encoded API token")
	)

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	blackboxExporterURL, err := url.Parse(*blackboxExporterStr)
	if err != nil {
		return err
	}

	if *apiToken == "" {
		return fmt.Errorf("invalid API token")
	}

	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, ctx := errgroup.WithContext(baseCtx)

	loggerWriter := ioutil.Discard
	if *verbose {
		loggerWriter = stdout
	}

	progname := filepath.Base(args[0])

	logger := log.New(loggerWriter, progname+": ", log.LstdFlags|log.Lmicroseconds)

	g.Go(func() error {
		return signalHandler(ctx, logger)
	})

	logger.Printf("Starting %s (%s, %s, %s)", progname, version, commit, buildstamp)

	promRegisterer := prometheus.NewRegistry()

	if err := registerMetrics(promRegisterer); err != nil {
		return err
	}

	router := NewMux(MuxOpts{
		Logger:         logger,
		PromRegisterer: promRegisterer,
	})

	httpConfig := http.Config{
		ListenAddr:   *httpListenAddr,
		Logger:       logger,
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

	checksUpdater, err := checks.NewUpdater(conn, *bbeConfigFilename, blackboxExporterURL, logger, publishCh, promRegisterer)
	if err != nil {
		log.Fatalf("Cannot create checks updater: %s", err)
	}

	g.Go(func() error {
		return checksUpdater.Run(ctx)
	})

	publisher := pusher.NewPublisher(conn, publishCh, logger, promRegisterer)

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

func signalHandler(ctx context.Context, logger *log.Logger) error {
	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Printf("Received signal %s. Shutting down.", sig)
		return fmt.Errorf("Got signal %s", sig)

	case <-ctx.Done():
		logger.Printf("Shutting down...")
		return nil
	}
}
