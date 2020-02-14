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

	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var exitError error

const exitFail = 1

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)

	var (
		verbose           = flags.Bool("verbose", false, "verbose logging")
		grpcApiServerAddr = flags.String("api-server-address", "localhost:4031", "GRPC API server address")
		grpcForwarderAddr = flags.String("forwarder-server-address", "localhost:4041", "GRPC forwarder server address")
		httpListenAddr    = flags.String("listen-address", ":4050", "listen address")
		probeName         = flags.String("probe-name", "probe-1", "name for this probe")
	)

	_ = grpcApiServerAddr
	_ = grpcForwarderAddr

	if err := flags.Parse(args[1:]); err != nil {
		return err
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

	publishCh := make(chan TimeSeries)

	config := config{
		forwarderAddress: *grpcForwarderAddr,
	}

	if err := envconfig.Process("worldping_test_metrics", &config.metrics); err != nil {
		log.Fatalf("Error obtaining metrics configuration from environment: %s", err)
	}

	if err := envconfig.Process("worldping_test_events", &config.events); err != nil {
		log.Fatalf("Error obtaining events configuration from environment: %s", err)
	}

	// TODO(mem): this is hacky.
	//
	// it's trying to deal with the fact that the URL shown to users
	// is not the push URL but the base for the API endpoints
	if u, err := url.Parse(config.events.URL + "/push"); err != nil {
		log.Fatalf("Invalid events push URL %q: %s", config.events.URL, err)
	} else {
		config.events.URL = u.String()
	}

	if u, err := url.Parse(config.metrics.URL + "/push"); err != nil {
		log.Fatalf("Invalid metrics push URL %q: %s", config.metrics.URL, err)
	} else {
		config.metrics.URL = u.String()
	}

	go publisher(ctx, publishCh, config, logger)


	<-ctx.Done()

	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer timeoutCancel()

	// we probably cannot do anything meaningful with this error
	_ = httpServer.Shutdown(timeoutCtx)

	return exitError
}

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "E: %s\n", err)
		os.Exit(exitFail)
	}
}

type config struct {
	forwarderAddress string

	metrics struct {
		Name     string `required:"true"`
		URL      string `required:"true"`
		Username string `required:"true"`
		Password string `required:"true"`
	}

	events struct {
		Name     string `required:"true"`
		URL      string `required:"true"`
		Username string `required:"true"`
		Password string `required:"true"`
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
