// Command otel-exporter is a proof-of-concept multi-tenant OTLP collector:
// it receives OTLP/HTTP traces on one listener, routes each request to a
// tenant by the bearer token presented in its Authorization header, and
// publishes the result to Tempo using that tenant's own endpoint and
// basic-auth credentials, all from a single process.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"gopkg.in/yaml.v3"
)

const defaultListenAddr = "localhost:4318"

// Tenant identifies one Tempo tenant's endpoint and basic-auth credentials.
// Password doubles as the bearer token producers use to authenticate to
// the collector's ingest side.
type Tenant struct {
	Name     string `yaml:"name"`
	Endpoint string `yaml:"endpoint"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Config is the top-level shape of the YAML config file.
type Config struct {
	ListenAddr string   `yaml:"listen_addr"`
	Tenants    []Tenant `yaml:"tenants"`
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}
	cfg := Config{
		ListenAddr: defaultListenAddr,
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cfg.ListenAddr == "" {
		return Config{}, fmt.Errorf("listen_addr must not be empty")
	}
	if len(cfg.Tenants) == 0 {
		return Config{}, fmt.Errorf("no tenants configured")
	}
	return cfg, nil
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to tenants YAML config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	factory := otlphttpexporter.NewFactory()

	exporters := make(map[string]exporter.Traces, len(cfg.Tenants))
	tokenToTenant := make(map[string]string, len(cfg.Tenants))
	for _, tenant := range cfg.Tenants {
		texp, err := buildTenantExporter(ctx, factory, tenant)
		if err != nil {
			slog.Error("failed to start tenant exporter", "tenant", tenant.Name, "error", err)
			os.Exit(1)
		}
		exporters[tenant.Name] = texp

		if existing, dup := tokenToTenant[tenant.Password]; dup {
			slog.Error("duplicate tenant token", "tenant", tenant.Name, "conflicts_with", existing)
			os.Exit(1)
		}
		tokenToTenant[tenant.Password] = tenant.Name
	}

	next, err := consumer.NewTraces(
		buildRouter(exporters, tokenToTenant),
		consumer.WithCapabilities(consumer.Capabilities{MutatesData: false}),
	)
	if err != nil {
		slog.Error("failed to build router", "error", err)
		os.Exit(1)
	}

	rcvr, err := buildReceiver(ctx, cfg.ListenAddr, next)
	if err != nil {
		slog.Error("failed to start collector", "error", err)
		os.Exit(1)
	}
	slog.Info("collector listening", "addr", cfg.ListenAddr)

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rcvr.Shutdown(shutdownCtx); err != nil {
		slog.Error("error shutting down collector", "error", err)
	}
	for name, texp := range exporters {
		if err := texp.Shutdown(shutdownCtx); err != nil {
			slog.Error("error shutting down exporter", "tenant", name, "error", err)
		}
	}

	slog.Info("shut down cleanly, exiting")
}
