package main

import (
	"context"
	"fmt"
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

var exitCode int

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	defer func() {
		cancel()
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	go signalHandler(ctx, cancel)

	logger := log.New(os.Stdout, "worldping-test: ", log.LstdFlags)

	logger.Printf("Starting worldping-test (%s, %s, %s)", version, commit, buildstamp)

	config := config{
		forwarderAddress: "localhost:4041",
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

	if err := registerMetrics(); err != nil {
		logger.Printf("cannot register prometheus metrics: %s", err)
		return
	}

	go runHttpServer(ctx, httpConfig{
		listenAddr:   "localhost:4040",
		logger:       logger,
		readTimeout:  5 * time.Second,
		writeTimeout: 10 * time.Second,
		idleTimeout:  15 * time.Second,
	}, cancel)

	go publishTestData(ctx, config, logger)

	<-ctx.Done()
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

func signalHandler(ctx context.Context, cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		exitCode = 1
		log.Printf("Received signal %s. shutting down", sig)

	case <-ctx.Done():
		log.Println("Shutting down")
	}

	cancel()
}

func runHttpServer(ctx context.Context, cfg httpConfig, cancel context.CancelFunc) {
	server := newHttpServer(ctx, cfg)

	server.ErrorLog.Println("Starting HTTP server...")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		server.ErrorLog.Printf("Could not listen on %s: %v\n", server.Addr, err)
		exitCode = 2
		cancel()
	}
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
