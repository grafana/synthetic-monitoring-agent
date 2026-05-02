// Command k6-runner is the server-side of the k6 runner service.
//
// It runs as either the dispatcher front-end (which agents POST /run
// to) or as a worker (which long-polls the dispatcher for jobs and
// executes them). The role is selected with the -role flag; both roles
// ship from the same binary so a single image can be deployed in two
// shapes.
//
// See docs/k6-runner-service-spec.md for the design.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/synthetic-monitoring-agent/internal/http"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner/dispatcher"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner/tiermap"
	k6version "github.com/grafana/synthetic-monitoring-agent/internal/k6runner/version"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner/worker"
	"github.com/grafana/synthetic-monitoring-agent/internal/version"
)

const (
	exitFail = 1

	roleDispatcher = "dispatcher"
	roleWorker     = "worker"
)

// runConfig is the parsed flag bag for the binary.
type runConfig struct {
	Role          string
	Debug         bool
	Verbose       bool
	ReportVersion bool
	ListenAddr    string
	EnablePProf   bool

	// shared between dispatcher and worker: the dispatcher serves
	// /versions and /versions/resolve from this repository, the worker
	// executes scripts with one of these binaries.
	K6URI        string
	K6Repository string

	// dispatcher-only
	Tiers            stringList
	Hold             time.Duration
	DequeueHold      time.Duration
	QueueCapacity    int
	TierMappingPath  string
	MappingReloadInt time.Duration

	// worker-only
	Tier            string
	DispatcherURL   string
	PollTimeout     time.Duration
	ResultTimeout   time.Duration
	K6BlacklistedIP string
}

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "E: %s\n", err)
		os.Exit(exitFail)
	}
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet(filepath.Base(args[0]), flag.ExitOnError)

	cfg := runConfig{
		Role:             roleDispatcher,
		ListenAddr:       "0.0.0.0:9090",
		Hold:             dispatcher.DefaultHold,
		DequeueHold:      dispatcher.DefaultDequeueHold,
		QueueCapacity:    dispatcher.DefaultQueueCapacity,
		MappingReloadInt: 30 * time.Second,
		PollTimeout:      worker.DefaultPollTimeout,
		ResultTimeout:    worker.DefaultResultTimeout,
		K6Repository:     "/usr/libexec/sm-k6",
		K6BlacklistedIP:  "10.0.0.0/8",
	}

	flags.StringVar(&cfg.Role, "role", cfg.Role, "process role: dispatcher or worker")
	flags.BoolVar(&cfg.Debug, "debug", cfg.Debug, "debug logging (implies -verbose)")
	flags.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "verbose logging")
	flags.BoolVar(&cfg.ReportVersion, "version", cfg.ReportVersion, "report version and exit")
	flags.StringVar(&cfg.ListenAddr, "listen-address", cfg.ListenAddr, "HTTP listen address")
	flags.BoolVar(&cfg.EnablePProf, "enable-pprof", cfg.EnablePProf, "expose /debug/pprof endpoints")

	flags.Var(&cfg.Tiers, "tiers", "(dispatcher) comma-separated list of deployed tier names, e.g. small,browser-A,browser-B")
	flags.DurationVar(&cfg.Hold, "hold", cfg.Hold, "(dispatcher) per-request hold while waiting for a worker")
	flags.DurationVar(&cfg.DequeueHold, "dequeue-hold", cfg.DequeueHold, "(dispatcher) /dequeue long-poll duration")
	flags.IntVar(&cfg.QueueCapacity, "queue-capacity", cfg.QueueCapacity, "(dispatcher) per-tier queue capacity")
	flags.StringVar(&cfg.TierMappingPath, "tier-mapping-path", cfg.TierMappingPath, "(dispatcher) path to tenant→tier YAML mapping")
	flags.DurationVar(&cfg.MappingReloadInt, "mapping-reload-interval", cfg.MappingReloadInt, "(dispatcher) interval at which the tier mapping file is checked for changes")

	flags.StringVar(&cfg.K6URI, "k6-uri", cfg.K6URI, "path or URI to a specific k6 binary; overrides version autodetection (used by dispatcher's /versions* and worker execution)")
	flags.StringVar(&cfg.K6Repository, "k6-repository", cfg.K6Repository, "path to folder containing k6 binaries (used by dispatcher's /versions* and worker execution)")

	flags.StringVar(&cfg.Tier, "tier", cfg.Tier, "(worker) tier name to pull from")
	flags.StringVar(&cfg.DispatcherURL, "dispatcher-url", cfg.DispatcherURL, "(worker) base URL of the dispatcher")
	flags.DurationVar(&cfg.PollTimeout, "poll-timeout", cfg.PollTimeout, "(worker) HTTP timeout for /dequeue long-polls")
	flags.DurationVar(&cfg.ResultTimeout, "result-timeout", cfg.ResultTimeout, "(worker) HTTP timeout for /result POSTs")
	flags.StringVar(&cfg.K6BlacklistedIP, "blocked-nets", cfg.K6BlacklistedIP, "(worker) IP networks to block in CIDR notation; empty disables")

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	if cfg.ReportVersion {
		fmt.Printf(
			"%s version=%q buildstamp=%q commit=%q\n",
			flags.Name(), version.Short(), version.Buildstamp(), version.Commit(),
		)
		return nil
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	zl := zerolog.New(stdout).With().Timestamp().Str("program", filepath.Base(args[0])).Logger()
	switch {
	case cfg.Debug:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		zl = zl.With().Caller().Logger()
	case cfg.Verbose:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	}

	zl.Info().Str("version", version.Short()).Str("commit", version.Commit()).
		Str("buildstamp", version.Buildstamp()).Str("role", cfg.Role).
		Msg("starting")

	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g, ctx := errgroup.WithContext(baseCtx)

	g.Go(func() error {
		return signalHandler(ctx, zl.With().Str("subsystem", "signal handler").Logger())
	})

	promReg := prometheus.NewRegistry()
	if err := registerMetrics(promReg); err != nil {
		return err
	}

	readyness := newReadynessHandler()

	switch cfg.Role {
	case roleDispatcher:
		if err := runDispatcher(ctx, g, &cfg, zl, promReg, readyness); err != nil {
			return err
		}
	case roleWorker:
		if err := runWorker(ctx, g, &cfg, zl, promReg, readyness); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown role %q (expected %q or %q)", cfg.Role, roleDispatcher, roleWorker)
	}

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		// signalHandler returns an error wrapping the signal name; that's expected and not a failure.
		var sigErr *signalError
		if errors.As(err, &sigErr) {
			return nil
		}
		return err
	}
	return nil
}

func runDispatcher(ctx context.Context, g *errgroup.Group, cfg *runConfig, zl zerolog.Logger,
	registerer *prometheus.Registry, readyness *readynessHandler) error {
	logger := zl.With().Str("subsystem", "dispatcher").Logger()

	if len(cfg.Tiers) == 0 {
		return errors.New("dispatcher: -tiers is required (e.g. -tiers=small,browser-A)")
	}

	mappingMetrics := tiermap.NewMetrics(registerer)
	dispatcherMetrics := dispatcher.NewMetrics(registerer)

	mapper, err := loadInitialMapping(cfg.TierMappingPath)
	if err != nil {
		return err
	}
	live := tiermap.NewLive(mapper, mappingMetrics, logger.With().Str("component", "tiermap").Logger())

	if cfg.TierMappingPath != "" {
		g.Go(func() error {
			live.Watch(ctx, cfg.TierMappingPath, cfg.MappingReloadInt)
			return nil
		})
	}

	// The dispatcher serves /versions and /versions/resolve so that agents
	// pointed at it can discover the k6 binaries the worker pool will
	// execute. Operators are expected to deploy the same set of binaries
	// to dispatcher and worker images. If neither -k6-repository nor
	// -k6-uri is set, /versions* return 503.
	var repo dispatcher.Repository

	if cfg.K6Repository != "" || cfg.K6URI != "" {
		r, err := k6version.NewRepository(cfg.K6Repository, cfg.K6URI)
		if err != nil {
			return fmt.Errorf("building k6 version repository: %w", err)
		}
		r.Logger = logger.With().Str("component", "k6-versions").Logger()
		repo = r
	}

	d, err := dispatcher.New(dispatcher.Config{
		Hold:          cfg.Hold,
		DequeueHold:   cfg.DequeueHold,
		QueueCapacity: cfg.QueueCapacity,
		Tiers:         []string(cfg.Tiers),
		Repository:    repo,
	}, live, dispatcherMetrics, logger)
	if err != nil {
		return err
	}

	mux := newMux(muxOpts{
		logger:       logger.With().Str("subsystem", "mux").Logger(),
		registerer:   registerer,
		readyness:    readyness,
		pprofEnabled: cfg.EnablePProf,
		extra:        d.Handler(),
	})

	httpServer := http.NewServer(ctx, mux, http.Config{
		ListenAddr:   cfg.ListenAddr,
		Logger:       logger.With().Str("subsystem", "http").Logger(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  120 * time.Second,
	})

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", httpServer.ListenAddr())
	if err != nil {
		return err
	}

	readyness.Set(true)

	g.Go(func() error {
		<-ctx.Done()
		// Drain queued jobs first so /run callers get the dispatcher_drain marker before we shut the server down.
		drainCtx, drainCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer drainCancel()
		d.Drain(drainCtx)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		return httpServer.Shutdown(shutdownCtx)
	})
	g.Go(func() error { return httpServer.Run(listener) })
	return nil
}

func loadInitialMapping(path string) (*tiermap.Mapper, error) {
	if path == "" {
		// Reasonable default for development / single-tier deployments: everything on small or browser-A.
		return tiermap.New([]byte(`
browser:
  default: browser-A
small:
  default: small
`))
	}
	b, err := os.ReadFile(path) //nolint:gosec // path is operator-supplied and trusted.
	if err != nil {
		return nil, fmt.Errorf("reading tier mapping: %w", err)
	}
	return tiermap.New(b)
}

func runWorker(ctx context.Context, g *errgroup.Group, cfg *runConfig, zl zerolog.Logger,
	registerer *prometheus.Registry, readyness *readynessHandler) error {
	logger := zl.With().Str("subsystem", "worker").Logger()

	if cfg.DispatcherURL == "" {
		return errors.New("worker: -dispatcher-url is required")
	}
	if cfg.Tier == "" {
		return errors.New("worker: -tier is required")
	}

	runner, err := k6runner.New(k6runner.RunnerOpts{
		Uri:           cfg.K6URI,
		Repository:    cfg.K6Repository,
		BlacklistedIP: cfg.K6BlacklistedIP,
		Registerer:    registerer,
	})
	if err != nil {
		return fmt.Errorf("building k6 runner: %w", err)
	}

	executor := worker.LocalExecutor{Runner: runner, Logger: logger.With().Str("component", "executor").Logger()}
	metrics := worker.NewMetrics(registerer)

	w, err := worker.New(worker.Config{
		DispatcherURL: cfg.DispatcherURL,
		Tier:          cfg.Tier,
		PollTimeout:   cfg.PollTimeout,
		ResultTimeout: cfg.ResultTimeout,
	}, executor, metrics, logger)
	if err != nil {
		return err
	}

	mux := newMux(muxOpts{
		logger:       logger.With().Str("subsystem", "mux").Logger(),
		registerer:   registerer,
		readyness:    readyness,
		pprofEnabled: cfg.EnablePProf,
	})
	httpServer := http.NewServer(ctx, mux, http.Config{
		ListenAddr:   cfg.ListenAddr,
		Logger:       logger.With().Str("subsystem", "http").Logger(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	})
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", httpServer.ListenAddr())
	if err != nil {
		return err
	}

	readyness.Set(true)

	g.Go(func() error {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		return httpServer.Shutdown(shutdownCtx)
	})
	g.Go(func() error { return httpServer.Run(listener) })
	g.Go(func() error { return w.Run(ctx) })
	return nil
}

type signalError struct{ sig string }

func (e *signalError) Error() string { return "got signal " + e.sig }

func signalHandler(ctx context.Context, logger zerolog.Logger) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		logger.Info().Str("signal", sig.String()).Msg("shutting down")
		return &signalError{sig: sig.String()}
	case <-ctx.Done():
		logger.Info().Msg("shutting down")
		return nil
	}
}

func registerMetrics(r prometheus.Registerer) error {
	for _, c := range []prometheus.Collector{
		collectors.NewBuildInfoCollector(),
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	} {
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}
