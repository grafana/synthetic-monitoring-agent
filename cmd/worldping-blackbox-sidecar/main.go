package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grafana/worldping-blackbox-sidecar/internal/checks"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pusher"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

var exitError error

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loggerWriter := ioutil.Discard
	if *verbose {
		loggerWriter = stdout
	}

	tag := fmt.Sprintf("%s: ", args[0])

	logger := log.New(loggerWriter, tag, log.LstdFlags)

	go signalHandler(ctx, cancel, logger)

	logger.Printf("Starting worldping-blackbox-sidecar (%s, %s, %s)", version, commit, buildstamp)

	if err := registerMetrics(); err != nil {
		return err
	}

	httpConfig := httpConfig{
		listenAddr:   *httpListenAddr,
		logger:       logger,
		readTimeout:  5 * time.Second,
		writeTimeout: 10 * time.Second,
		idleTimeout:  15 * time.Second,
	}

	httpServer := newHttpServer(ctx, httpConfig)

	go func(cancel context.CancelFunc) {
		var lc net.ListenConfig

		l, err := lc.Listen(ctx, "tcp", httpServer.Addr)
		if err != nil {
			exitError = err
			cancel()
			return
		}

		if err := runHttpServer(l, httpServer); err != nil {
			exitError = err
			cancel()
			return
		}
	}(cancel)

	publishCh := make(chan pusher.Payload)

	apiCreds := creds{Token: *apiToken}

	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithPerRPCCredentials(apiCreds),
	}

	if *grpcInsecure {
		opts = append(opts, grpc.WithInsecure())
	}

	conn, err := grpc.DialContext(ctx, *grpcApiServerAddr, opts...)
	if err != nil {
		return fmt.Errorf("dialing GRPC server %s: %w", *grpcApiServerAddr, err)
	}
	defer conn.Close()

	checksUpdater, err := checks.NewUpdater(conn, *bbeConfigFilename, blackboxExporterURL, logger, publishCh)
	if err != nil {
		log.Fatalf("Cannot create checks updater: %s", err)
	}

	go func() {
		if err := checksUpdater.Run(ctx); err != nil {
			logger.Printf("E: while running checks updater: %s", err)
			cancel()
		}
	}()

	publisher := pusher.NewPublisher(conn, publishCh, logger)

	go func() {
		if err := publisher.Run(ctx); err != nil {
			// we should never see this, if we are here
			// something bad happened
			logger.Printf("E: while running publisher: %s", err)
			cancel()
		}
	}()

	<-ctx.Done()

	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer timeoutCancel()

	// we probably cannot do anything meaningful with this error
	_ = httpServer.Shutdown(timeoutCtx)

	return exitError
}

type creds struct {
	Token string
}

func (c creds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + c.Token,
	}, nil
}

func (c creds) RequireTransportSecurity() bool {
	log.Printf("RequireTransportSecurity")
	// XXX(mem): this is true
	return false
}

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "E: %s\n", err)
		os.Exit(exitFail)
	}
}

type httpConfig struct {
	listenAddr   string
	logger       *log.Logger
	readTimeout  time.Duration
	writeTimeout time.Duration
	idleTimeout  time.Duration
}

func signalHandler(ctx context.Context, cancel context.CancelFunc, logger *log.Logger) {
	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		exitError = fmt.Errorf("Got signal %s", sig)
		logger.Printf("Received signal %s. Shutting down.", sig)

	case <-ctx.Done():
		logger.Printf("Shutting down...")
	}

	cancel()
}

func defaultHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusOK)

		fmt.Fprintln(w, "hello, world!")
	})
}

func newHttpServer(ctx context.Context, cfg httpConfig) *http.Server {
	router := http.NewServeMux()
	router.Handle("/", defaultHandler())
	router.Handle("/metrics", promhttp.Handler())

	return &http.Server{
		Addr:         cfg.listenAddr,
		Handler:      router,
		ErrorLog:     cfg.logger,
		ReadTimeout:  cfg.readTimeout,
		WriteTimeout: cfg.writeTimeout,
		IdleTimeout:  cfg.idleTimeout,
		BaseContext:  func(net.Listener) context.Context { return ctx },
	}
}

func runHttpServer(l net.Listener, server *http.Server) error {
	server.ErrorLog.Printf("Starting HTTP server on %s...", server.Addr)

	err := server.Serve(l)
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("could not serve HTTP on %s: %w", server.Addr, err)
	}

	return nil
}
