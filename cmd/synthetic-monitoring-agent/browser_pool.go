package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/browser"
	"github.com/grafana/synthetic-monitoring-agent/internal/discovery"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
)

// browserPoolConfig groups the -browser-pool-* flags.
type browserPoolConfig struct {
	Enabled   bool
	Addresses StringList
}

// validateBrowserPoolConfig rejects -browser-pool-enabled without addresses, and
// addresses without -browser-pool-enabled.
func validateBrowserPoolConfig(cfg browserPoolConfig) error {
	switch {
	case cfg.Enabled && len(cfg.Addresses) == 0:
		return errors.New("-browser-pool-enabled requires -browser-pool-addresses")
	case !cfg.Enabled && len(cfg.Addresses) > 0:
		return errors.New("-browser-pool-addresses requires -browser-pool-enabled")
	}

	return nil
}

// buildBrowserPool translates the -browser-pool-* flags into a running
// browser.Pool, whose sync loop stops when ctx is cancelled. It returns the
// k6runner interface type so a typed-nil can never reach
// RunnerOpts.BrowserPool.
func buildBrowserPool(
	ctx context.Context, addresses []string, logger zerolog.Logger, registerer prometheus.Registerer,
) (k6runner.BrowserPool, error) {
	discoverFn, err := discovery.NewDiscoverer(addresses, browser.DefaultInstancePort, logger)
	if err != nil {
		return nil, fmt.Errorf("configuring browser pool discovery: %w", err)
	}

	pool, err := browser.New(ctx, browser.Config{
		Discover: discoverFn,
		Logger:   logger,
	}, registerer)
	if err != nil {
		return nil, err
	}
	return pool, nil
}
