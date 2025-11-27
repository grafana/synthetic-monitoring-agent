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
	"strconv"
	"syscall"
	"time"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/google/uuid"
	"github.com/jpillora/backoff"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/grpclog"

	"github.com/grafana/synthetic-monitoring-agent/internal/adhoc"
	"github.com/grafana/synthetic-monitoring-agent/internal/cache"
	"github.com/grafana/synthetic-monitoring-agent/internal/cals"
	"github.com/grafana/synthetic-monitoring-agent/internal/checks"
	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/http"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/limits"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	pusherV1 "github.com/grafana/synthetic-monitoring-agent/internal/pusher/v1"
	pusherV2 "github.com/grafana/synthetic-monitoring-agent/internal/pusher/v2"
	"github.com/grafana/synthetic-monitoring-agent/internal/scraper"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
	"github.com/grafana/synthetic-monitoring-agent/internal/telemetry"
	"github.com/grafana/synthetic-monitoring-agent/internal/tenants"
	"github.com/grafana/synthetic-monitoring-agent/internal/usage"
	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

const (
	exitFail             = 1
	defTelemetryTimeSpan = 5 // min
)

// run is the main entry point for the program.
//
// TODO(mem): refactor this function to be more readable.
//
//nolint:gocyclo // this function is doing a lot of configuration, and it ends up being long and with lots of branches.
func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet(filepath.Base(args[0]), flag.ExitOnError)

	var (
		features = feature.NewCollection()
		config   = struct {
			DevMode               bool
			Debug                 bool
			Verbose               bool
			ReportVersion         bool
			GrpcApiServerAddr     string
			GrpcInsecure          bool
			ApiToken              Secret
			EnableChangeLogLevel  bool
			EnableDisconnect      bool
			EnablePProf           bool
			HttpListenAddr        string
			K6URI                 string
			K6BlacklistedIP       string
			SelectedPublisher     string
			TelemetryTimeSpan     int
			AutoMemLimit          bool
			MemLimitRatio         float64
			DisableK6             bool
			DisableUsageReports   bool
			CacheType             cache.Kind
			CacheLocalCapacity    int
			CacheLocalTTL         time.Duration
			MemcachedServers      StringList
			EnableProtocolSecrets bool
		}{
			GrpcApiServerAddr:  "localhost:4031",
			HttpListenAddr:     "localhost:4050",
			K6URI:              "sm-k6",
			K6BlacklistedIP:    "10.0.0.0/8",
			SelectedPublisher:  pusherV2.Name,
			TelemetryTimeSpan:  defTelemetryTimeSpan,
			AutoMemLimit:       true,
			MemLimitRatio:      0.9,
			CacheType:          cache.KindAuto,
			CacheLocalCapacity: 10000,
			CacheLocalTTL:      5 * time.Minute,
		}
	)

	flags.BoolVar(&config.DevMode, "dev", config.DevMode, "turn on all development flags")
	flags.BoolVar(&config.Debug, "debug", config.Debug, "debug output (enables verbose)")
	flags.BoolVar(&config.Verbose, "verbose", config.Verbose, "verbose logging")
	flags.BoolVar(&config.ReportVersion, "version", config.ReportVersion, "report version and exit")
	flags.StringVar(&config.GrpcApiServerAddr, "api-server-address", config.GrpcApiServerAddr, "GRPC API server address")
	flags.BoolVar(&config.GrpcInsecure, "api-insecure", config.GrpcInsecure, "Don't use TLS with connections to GRPC API")
	flags.Var(&config.ApiToken, "api-token", `synthetic monitoring probe authentication token (default "")`)
	flags.BoolVar(&config.EnableChangeLogLevel, "enable-change-log-level", config.EnableChangeLogLevel, "enable changing the log level at runtime")
	flags.BoolVar(&config.EnableDisconnect, "enable-disconnect", config.EnableDisconnect, "enable HTTP /disconnect endpoint")
	flags.BoolVar(&config.EnablePProf, "enable-pprof", config.EnablePProf, "exposes profiling data via HTTP /debug/pprof/ endpoint")
	flags.StringVar(&config.HttpListenAddr, "listen-address", config.HttpListenAddr, "listen address")
	flags.StringVar(&config.K6URI, "k6-uri", config.K6URI, "how to run k6 (path or URL)")
	flags.StringVar(&config.K6BlacklistedIP, "blocked-nets", config.K6BlacklistedIP,
		"IP networks to block in CIDR notation. Setting this to an empty string, or '0.0.0.0/32', will disable the blocklist.")
	flags.StringVar(&config.SelectedPublisher, "publisher", config.SelectedPublisher, "publisher type")
	flags.IntVar(&config.TelemetryTimeSpan, "telemetry-time-span", config.TelemetryTimeSpan, "time span between telemetry push executions per tenant")
	flags.BoolVar(&config.AutoMemLimit, "enable-auto-memlimit", config.AutoMemLimit, "automatically set GOMEMLIMIT")
	flags.BoolVar(&config.DisableK6, "disable-k6", config.DisableK6, "disables running k6 checks on this probe")
	flags.BoolVar(&config.DisableUsageReports, "disable-usage-reports", config.DisableUsageReports, "Disable anonymous usage reports")
	flags.Float64Var(&config.MemLimitRatio, "memlimit-ratio", config.MemLimitRatio, "fraction of available memory to use")
	flags.Var(&features, "features", "optional feature flags")
	flags.Var(&config.CacheType, "cache-type", "cache type: auto (memcached if servers provided, else local), memcached, local, or noop")
	flags.IntVar(&config.CacheLocalCapacity, "cache-local-capacity", config.CacheLocalCapacity, "maximum number of items in local cache")
	flags.DurationVar(&config.CacheLocalTTL, "cache-local-ttl", config.CacheLocalTTL, "default TTL for local cache items")
	flags.Var(&config.MemcachedServers, "memcached-servers", "memcached servers")

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	if config.ReportVersion {
		fmt.Printf(
			"%s version=\"%s\" buildstamp=\"%s\" commit=\"%s\"\n",
			flags.Name(),
			version.Short(),
			version.Buildstamp(),
			version.Commit(),
		)
		return nil
	}

	if config.DevMode {
		config.Debug = true
		config.EnableChangeLogLevel = true
		config.EnableDisconnect = true
		config.EnablePProf = true
	}

	if config.AutoMemLimit {
		err := setupGoMemLimit(config.MemLimitRatio)
		if err != nil {
			return err
		}
	}

	if !config.DisableK6 {
		if err := features.Set(feature.K6); err != nil {
			return fmt.Errorf("cannot set k6 feature: %w", err)
		}
	}

	if _, _, err := net.SplitHostPort(config.GrpcApiServerAddr); err != nil {
		// SplitHostPort errors if the address has no port. This is intended, as omitting the port in the address is
		// almost likely a user error that is hard to troubleshoot otherwise.
		return fmt.Errorf("parsing GRPC api server address %q: %w", config.GrpcApiServerAddr, err)
	}

	// If the token is provided on the command line, prefer that. Otherwise
	// pull it from the environment variable SM_AGENT_API_TOKEN. If that's
	// not available, fallback to API_TOKEN, which was the environment
	// variable name previously used in the systemd unit files.
	//
	// Using API_TOKEN should be deprecated after March 1st, 2023.
	config.ApiToken = Secret(stringFromEnv("API_TOKEN", stringFromEnv("SM_AGENT_API_TOKEN", string(config.ApiToken))))

	// Enable protocol secrets support via environment variable.
	// This is a feature flag to allow testing before enabling by default.
	config.EnableProtocolSecrets = boolFromEnv("SM_AGENT_ENABLE_PROTOCOL_SECRETS", config.EnableProtocolSecrets)

	if config.ApiToken == "" {
		return fmt.Errorf("invalid API token")
	}

	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, ctx := errgroup.WithContext(baseCtx)

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs

	zl := zerolog.New(stdout).With().Timestamp().Str("program", filepath.Base(args[0])).Logger()

	switch {
	case config.Debug:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		zlGrpc := zl.With().Str("component", "grpc-go").Logger()
		zl = zl.With().Caller().Logger()
		config.Verbose = true
		grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(zlGrpc, zlGrpc, zlGrpc, 99))

	case config.Verbose:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)

	default:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	}

	g.Go(func() error {
		return signalHandler(ctx, zl.With().Str("subsystem", "signal handler").Logger())
	})

	zl.Info().
		Str("version", version.Short()).
		Str("commit", version.Commit()).
		Str("buildstamp", version.Buildstamp()).
		Str("features", features.String()).
		Interface("config", config).
		Msg("starting")

	notifyAboutDeprecatedFeatureFlags(features, zl)

	if features.IsSet(feature.K6) {
		newUri, err := validateK6URI(config.K6URI)
		if err != nil {
			return err
		} else if newUri != config.K6URI {
			config.K6URI = newUri
		}
	} else {
		config.K6URI = ""
	}

	if len(config.K6URI) > 0 {
		zl.Info().Str("k6URI", config.K6URI).Msg("enabling k6 checks")
	} else {
		zl.Info().Msg("disabling k6 checks")
	}

	var usageReporter usage.Reporter
	switch {
	case config.DisableUsageReports:
		usageReporter = usage.NewNoOPReporter()
	default:
		usageReporter = usage.NewHTTPReporter(usage.ProdStatsEndpoint)
		zl.Info().Msg("enabled collecting anonymous usage reports. You can disable collection by setting -disable-usage-reports.")
	}

	promRegisterer := prometheus.NewRegistry()

	if err := registerMetrics(promRegisterer); err != nil {
		return err
	}

	// Initialize cache client (always non-nil, with fallback chain: memcached → local → noop)
	cacheClient := setupCache(
		config.CacheType,
		config.MemcachedServers,
		config.CacheLocalCapacity,
		config.CacheLocalTTL,
		&zl,
	)

	// to know if probe is connected to API
	readynessHandler := NewReadynessHandler()

	router := NewMux(MuxOpts{
		Logger:                zl.With().Str("subsystem", "mux").Logger(),
		PromRegisterer:        promRegisterer,
		isReady:               readynessHandler,
		changeLogLevelEnabled: config.EnableChangeLogLevel,
		disconnectEnabled:     config.EnableDisconnect,
		pprofEnabled:          config.EnablePProf,
	})

	httpConfig := http.Config{
		ListenAddr:   config.HttpListenAddr,
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

	conn, err := dialAPIServer(ctx, config.GrpcApiServerAddr, config.GrpcInsecure, string(config.ApiToken))
	if err != nil {
		return fmt.Errorf("dialing GRPC server %s: %w", config.GrpcApiServerAddr, err)
	}
	defer conn.Close()

	var k6Runner k6runner.Runner
	if features.IsSet(feature.K6) && len(config.K6URI) > 0 {
		if err := validateCIDR(config.K6BlacklistedIP); err != nil {
			return err
		}

		k6Runner = k6runner.New(k6runner.RunnerOpts{
			Uri:           config.K6URI,
			BlacklistedIP: config.K6BlacklistedIP,
			Registerer:    promRegisterer,
		})
	}

	tm := tenants.NewManager(
		ctx,
		synthetic_monitoring.NewTenantsClient(conn),
		tenantCh,
		tenants.DefaultCacheTimeout,
		cacheClient,
		zl.With().Str("subsystem", "tenant_manager").Logger(),
	)

	pusherRegistry := pusher.NewRegistry[pusher.Factory]()
	pusherRegistry.MustRegister(pusherV1.Name, pusherV1.NewPublisher)
	pusherRegistry.MustRegister(pusherV2.Name, pusherV2.NewPublisher)

	publisherFactory, err := pusherRegistry.Lookup(config.SelectedPublisher)
	if err != nil {
		return fmt.Errorf("creating publisher: %w", err)
	}

	publisher := publisherFactory(ctx, tm, zl.With().Str("subsystem", "publisher").Str("version", config.SelectedPublisher).Logger(), promRegisterer)
	limits := limits.NewTenantLimits(tm)
	secretProvider := secrets.NewSecretProvider(tm, 60*time.Second, zl.With().Str("subsystem", "secretstore").Logger())
	cals := cals.NewCostAttributionLabels(tm)

	telemetry := telemetry.NewTelemeter(
		ctx, uuid.New().String(), time.Duration(config.TelemetryTimeSpan)*time.Minute,
		synthetic_monitoring.NewTelemetryClient(conn),
		zl.With().Str("subsystem", "telemetry").Logger(),
		promRegisterer,
	)

	checksUpdater, err := checks.NewUpdater(checks.UpdaterOptions{
		Conn:                    conn,
		Logger:                  zl.With().Str("subsystem", "updater").Logger(),
		Backoff:                 newConnectionBackoff(),
		Publisher:               publisher,
		TenantCh:                tenantCh,
		IsConnected:             readynessHandler.Set,
		PromRegisterer:          promRegisterer,
		Features:                features,
		K6Runner:                k6Runner,
		ScraperFactory:          scraper.New,
		TenantLimits:            limits,
		SecretProvider:          secretProvider,
		Telemeter:               telemetry,
		UsageReporter:           usageReporter,
		CostAttributionLabels:   cals,
		SupportsProtocolSecrets: config.EnableProtocolSecrets,
	})

	if err != nil {
		return fmt.Errorf("cannot create checks updater: %w", err)
	}

	g.Go(func() error {
		return checksUpdater.Run(ctx)
	})

	adhocHandler, err := adhoc.NewHandler(adhoc.HandlerOpts{
		Conn:                    conn,
		Logger:                  zl.With().Str("subsystem", "adhoc").Logger(),
		Backoff:                 newConnectionBackoff(),
		Publisher:               publisher,
		TenantCh:                tenantCh,
		PromRegisterer:          promRegisterer,
		Features:                features,
		K6Runner:                k6Runner,
		SecretProvider:          secretProvider,
		SupportsProtocolSecrets: config.EnableProtocolSecrets,
	})
	if err != nil {
		return fmt.Errorf("cannot create ad-hoc checks handler: %w", err)
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
		return fmt.Errorf("got signal %s", sig)

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

func boolFromEnv(name string, defaultValue bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		// If parsing fails, return the default value
		return defaultValue
	}

	return parsed
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
	for _, ff := range []string{feature.K6, feature.Traceroute} {
		if features.IsSet(ff) {
			zl.Info().Msgf("the `%s` feature is now permanently enabled in the agent, you can remove it from the -features flag without loss of functionality", ff)
		}
	}
}

func setupGoMemLimit(ratio float64) error {
	_, err := memlimit.SetGoMemLimitWithOpts(
		memlimit.WithRatio(ratio),
		memlimit.WithProvider(
			memlimit.ApplyFallback(
				memlimit.FromCgroup, // prefer cgroup limit if available
				memlimit.FromSystem, // fallback to the system's memory
			),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to set GOMEMLIMIT: %w", err)
	}

	return nil
}

func setupCache(cacheType cache.Kind, memcachedServers []string, localCapacity int, localTTL time.Duration, logger *zerolog.Logger) cache.Cache {
	// Determine effective cache type with auto mode logic:
	// auto + servers provided -> memcached -> local -> noop
	// auto + no servers -> local -> noop
	effectiveType := cacheType
	if cacheType == cache.KindAuto {
		if len(memcachedServers) > 0 {
			effectiveType = cache.KindMemcached
		} else {
			effectiveType = cache.KindLocal
		}
	}

	switch effectiveType {
	case cache.KindMemcached:
		if len(memcachedServers) == 0 {
			logger.Warn().Msg("memcached type selected but no servers configured, falling back to local cache")
			return setupLocalCache(localCapacity, localTTL, logger)
		}

		cacheConfig := cache.MemcachedConfig{
			Servers: memcachedServers,
			Logger:  logger.With().Str("subsystem", "cache").Logger(),
			Timeout: 100 * time.Millisecond,
		}

		cacheClient, err := cache.NewMemcachedClient(cacheConfig)
		if err != nil {
			logger.Warn().
				Err(err).
				Strs("servers", memcachedServers).
				Msg("failed to initialize memcached cache, falling back to local cache")
			return setupLocalCache(localCapacity, localTTL, logger)
		}

		logger.Info().
			Strs("servers", memcachedServers).
			Msg("memcached cache initialized")
		return cacheClient

	case cache.KindLocal:
		return setupLocalCache(localCapacity, localTTL, logger)

	case cache.KindNoop:
		logger.Debug().Msg("noop cache selected")
		return cache.NewNoop(logger.With().Str("subsystem", "cache").Logger())

	default:
		logger.Warn().
			Stringer("type", effectiveType).
			Msg("unknown cache type, falling back to local cache")

		return setupLocalCache(localCapacity, localTTL, logger)
	}
}

func setupLocalCache(capacity int, ttl time.Duration, logger *zerolog.Logger) cache.Cache {
	localConfig := cache.LocalConfig{
		MaxCapacity:     capacity,
		InitialCapacity: capacity / 10, // 10% of max as initial capacity
		DefaultTTL:      ttl,
		Logger:          logger.With().Str("subsystem", "cache").Logger(),
	}

	localCache, err := cache.NewLocal(localConfig)
	if err != nil {
		logger.Warn().
			Err(err).
			Int("capacity", capacity).
			Dur("ttl", ttl).
			Msg("failed to initialize local cache, using noop cache")
		return cache.NewNoop(logger.With().Str("subsystem", "cache").Logger())
	}

	logger.Info().
		Int("capacity", capacity).
		Dur("ttl", ttl).
		Msg("local cache initialized")
	return localCache
}
