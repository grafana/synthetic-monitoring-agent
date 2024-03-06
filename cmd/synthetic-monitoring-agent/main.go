package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jpillora/backoff"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/grpclog"

	"github.com/grafana/synthetic-monitoring-agent/internal/adhoc"
	"github.com/grafana/synthetic-monitoring-agent/internal/checks"
	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/http"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/limits"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	pusherV1 "github.com/grafana/synthetic-monitoring-agent/internal/pusher/v1"
	pusherV2 "github.com/grafana/synthetic-monitoring-agent/internal/pusher/v2"
	"github.com/grafana/synthetic-monitoring-agent/internal/scraper"
	"github.com/grafana/synthetic-monitoring-agent/internal/telemetry"
	"github.com/grafana/synthetic-monitoring-agent/internal/tenants"
	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

const (
	exitFail             = 1
	defTelemetryTimeSpan = 5 // min
)

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet(filepath.Base(args[0]), flag.ExitOnError)

	var (
		features             = feature.NewCollection()
		devMode              = flags.Bool("dev", false, "turn on all development flags")
		debug                = flags.Bool("debug", false, "debug output (enables verbose)")
		verbose              = flags.Bool("verbose", false, "verbose logging")
		reportVersion        = flags.Bool("version", false, "report version and exit")
		grpcApiServerAddr    = flags.String("api-server-address", "localhost:4031", "GRPC API server address")
		grpcInsecure         = flags.Bool("api-insecure", false, "Don't use TLS with connections to GRPC API")
		apiToken             = flags.String("api-token", "", "synthetic monitoring probe authentication token")
		enableChangeLogLevel = flags.Bool("enable-change-log-level", false, "enable changing the log level at runtime")
		enableDisconnect     = flags.Bool("enable-disconnect", false, "enable HTTP /disconnect endpoint")
		enablePProf          = flags.Bool("enable-pprof", false, "exposes profiling data via HTTP /debug/pprof/ endpoint")
		httpListenAddr       = flags.String("listen-address", "localhost:4050", "listen address")
		k6URI                = flags.String("k6-uri", "k6", "how to run k6 (path or URL)")
		k6BlacklistedIP      = flags.String("blocked-nets", "10.0.0.0/8", "IP networks to block in CIDR notation, disabled if empty")
		selectedPublisher    = flags.String("publisher", pusherV1.Name, "publisher type (EXPERIMENTAL)")
		telemetryTimeSpan    = flags.Int("telemetry-time-span", defTelemetryTimeSpan, "time span between telemetry push executions per tenant")
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

	if *devMode {
		*debug = true
		*enableChangeLogLevel = true
		*enableDisconnect = true
		*enablePProf = true
	}

	// If the token is provided on the command line, prefer that. Otherwise
	// pull it from the environment variable SM_AGENT_API_TOKEN. If that's
	// not available, fallback to API_TOKEN, which was the environment
	// variable name previously used in the systemd unit files.
	//
	// Using API_TOKEN should be deprecated after March 1st, 2023.
	*apiToken = stringFromEnv("API_TOKEN", stringFromEnv("SM_AGENT_API_TOKEN", *apiToken))

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
		zlGrpc := zl.With().Str("component", "grpc-go").Logger()
		zl = zl.With().Caller().Logger()
		*verbose = true
		grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(zlGrpc, zlGrpc, zlGrpc, 99))

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
		Bool("change-log-level-enabled", *enableChangeLogLevel).
		Bool("disconnect-enabled", *enableDisconnect).
		Bool("pprof-enabled", *enablePProf).
		Msg("starting")

	notifyAboutDeprecatedFeatureFlags(features, zl)

	if features.IsSet(feature.K6) {
		newUri, err := validateK6URI(*k6URI)
		if err != nil {
			*k6URI = ""
			zl.Warn().Str("k6URI", *k6URI).Err(err).Msg("invalid k6 URI")
		} else if newUri != *k6URI {
			*k6URI = newUri
		}
	} else {
		*k6URI = ""
	}

	if len(*k6URI) > 0 {
		zl.Info().Str("k6URI", *k6URI).Msg("enabling k6 checks")
	} else {
		zl.Info().Msg("disabling k6 checks")
	}

	promRegisterer := prometheus.NewRegistry()

	if err := registerMetrics(promRegisterer); err != nil {
		return err
	}

	// to know if probe is connected to API
	readynessHandler := NewReadynessHandler()

	router := NewMux(MuxOpts{
		Logger:                zl.With().Str("subsystem", "mux").Logger(),
		PromRegisterer:        promRegisterer,
		isReady:               readynessHandler,
		changeLogLevelEnabled: *enableChangeLogLevel,
		disconnectEnabled:     *enableDisconnect,
		pprofEnabled:          *enablePProf,
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

	tenantCh := make(chan synthetic_monitoring.Tenant)

	conn, err := dialAPIServer(ctx, *grpcApiServerAddr, *grpcInsecure, *apiToken)
	if err != nil {
		return fmt.Errorf("dialing GRPC server %s: %w", *grpcApiServerAddr, err)
	}
	defer conn.Close()

	var k6Runner k6runner.Runner
	if features.IsSet(feature.K6) && len(*k6URI) > 0 {
		if err := validateCIDR(*k6BlacklistedIP); err != nil {
			return err
		}

		k6Runner = k6runner.New(k6runner.RunnerOpts{
			Uri:           *k6URI,
			BlacklistedIP: *k6BlacklistedIP,
		})
	}

	tm := tenants.NewManager(ctx, synthetic_monitoring.NewTenantsClient(conn), tenantCh, 15*time.Minute)

	pusherRegistry := pusher.NewRegistry[pusher.Factory]()
	pusherRegistry.MustRegister(pusherV1.Name, pusherV1.NewPublisher)
	pusherRegistry.MustRegister(pusherV2.Name, pusherV2.NewPublisher)

	publisherFactory, err := pusherRegistry.Lookup(*selectedPublisher)
	if err != nil {
		return fmt.Errorf("creating publisher: %w", err)
	}

	publisher := publisherFactory(ctx, tm, zl.With().Str("subsystem", "publisher").Str("version", *selectedPublisher).Logger(), promRegisterer)
	limits := limits.NewTenantLimits(tm)

	telemetry := telemetry.NewTelemeter(
		ctx, uuid.New().String(), time.Duration(*telemetryTimeSpan)*time.Minute,
		synthetic_monitoring.NewTelemetryClient(conn),
		zl.With().Str("subsystem", "telemetry").Logger(),
		promRegisterer,
	)

	checksUpdater, err := checks.NewUpdater(checks.UpdaterOptions{
		Conn:           conn,
		Logger:         zl.With().Str("subsystem", "updater").Logger(),
		Backoff:        newConnectionBackoff(),
		Publisher:      publisher,
		TenantCh:       tenantCh,
		IsConnected:    readynessHandler.Set,
		PromRegisterer: promRegisterer,
		Features:       features,
		K6Runner:       k6Runner,
		ScraperFactory: scraper.New,
		TenantLimits:   limits,
		Telemeter:      telemetry,
	})
	if err != nil {
		return fmt.Errorf("Cannot create checks updater: %w", err)
	}

	g.Go(func() error {
		return checksUpdater.Run(ctx)
	})

	adhocHandler, err := adhoc.NewHandler(adhoc.HandlerOpts{
		Conn:           conn,
		Logger:         zl.With().Str("subsystem", "adhoc").Logger(),
		Backoff:        newConnectionBackoff(),
		Publisher:      publisher,
		TenantCh:       tenantCh,
		PromRegisterer: promRegisterer,
		Features:       features,
		K6Runner:       k6Runner,
	})
	if err != nil {
		return fmt.Errorf("Cannot create ad-hoc checks handler: %w", err)
	}

	g.Go(func() error {
		return adhocHandler.Run(ctx)
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

func newConnectionBackoff() *backoff.Backoff {
	return &backoff.Backoff{
		Min:    2 * time.Second,
		Max:    30 * time.Second,
		Factor: math.Pow(30./2., 1./8.), // reach the target in ~ 8 steps
		Jitter: true,
	}
}

func validateCIDR(ip string) error {
	if ip != "" {
		if _, _, err := net.ParseCIDR(ip); err != nil {
			return err
		}
	}

	return nil
}

func stringFromEnv(name string, override string) string {
	if override != "" {
		return override
	}

	return os.Getenv(name)
}

func validateK6URI(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	switch u.Scheme {
	case "http", "https":

	case "":
		if u.Path == "" {
			return "", fmt.Errorf("missing path in %q", uri)
		}

		uri, err = exec.LookPath(u.Path)
		if err != nil {
			return "", err
		}

	default:
		return "", fmt.Errorf("invalid scheme %q", u.Scheme)
	}

	return uri, nil
}

func notifyAboutDeprecatedFeatureFlags(features feature.Collection, zl zerolog.Logger) {
	for _, ff := range []string{feature.AdHoc, feature.Traceroute} {
		if features.IsSet(ff) {
			zl.Info().Msgf("the `%s` feature is now permanently enabled in the agent, you can remove it from the --feature flag without loss of functionality", ff)
		}
	}
}
