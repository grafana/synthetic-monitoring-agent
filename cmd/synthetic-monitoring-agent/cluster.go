package main

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/cluster"
	"github.com/grafana/synthetic-monitoring-agent/internal/discovery"
)

// clusterConfig groups the -cluster-* flags that configure gossip-based check
// ownership. See buildClusterNode for how they map onto cluster.RingConfig.
type clusterConfig struct {
	Enabled                bool
	NodeName               string
	AdvertiseAddress       string
	AdvertiseInterfaces    StringList
	ListenPort             int
	Label                  string
	JoinAddresses          StringList
	MinimumSize            int
	MinimumSizeWaitTimeout time.Duration
	RejoinInterval         time.Duration
}

// buildClusterNode constructs the gossip ring node from the cluster flags. It
// only constructs it — it does not start gossip or join the cluster; the caller
// does that via RingNode.Start.
func buildClusterNode(cfg clusterConfig, nodeName string, logger zerolog.Logger, registerer prometheus.Registerer) (*cluster.RingNode, error) {
	advertiseAddr, err := clusterAdvertiseAddr(cfg.AdvertiseAddress, cfg.AdvertiseInterfaces, cfg.ListenPort)
	if err != nil {
		return nil, fmt.Errorf("resolving cluster advertise address: %w", err)
	}

	discoverFn, err := discovery.NewDiscoverer(cfg.JoinAddresses, cfg.ListenPort, logger)
	if err != nil {
		return nil, fmt.Errorf("configuring cluster peer discovery: %w", err)
	}

	node, err := cluster.NewRingNode(cluster.RingConfig{
		Name:                   nodeName,
		AdvertiseAddr:          advertiseAddr,
		Label:                  cfg.Label,
		Client:                 cluster.NewGossipClient(),
		Discover:               discoverFn,
		Logger:                 logger,
		RejoinInterval:         cfg.RejoinInterval,
		MinimumClusterSize:     cfg.MinimumSize,
		MinimumSizeWaitTimeout: cfg.MinimumSizeWaitTimeout,
	}, registerer)
	if err != nil {
		return nil, fmt.Errorf("creating cluster node: %w", err)
	}

	return node, nil
}

// resolveClusterNodeName returns the configured node name, falling back to the
// hostname (the stable pod name in Kubernetes) when unset. If the hostname
// cannot be resolved, it returns a generated name and the resolution error.
func resolveClusterNodeName(name string, hostname func() (string, error)) (string, error) {
	if name != "" {
		return name, nil
	}

	resolvedName, err := hostname()
	if err == nil && resolvedName != "" {
		return resolvedName, nil
	}

	if err == nil {
		err = errors.New("hostname is empty")
	}

	return uuid.New().String(), err
}

// clusterAdvertiseAddr returns the explicit advertise address when set, adding
// the gossip port if it has none; otherwise it resolves one from the given
// interfaces and gossip port.
func clusterAdvertiseAddr(explicit string, interfaces []string, port int) (string, error) {
	if explicit == "" {
		return cluster.AdvertiseAddress(interfaces, port)
	}

	_, p, err := net.SplitHostPort(explicit)
	if err != nil {
		// No port. Trim brackets from an IPv6 address like "[::1]": JoinHostPort
		// adds its own, which would otherwise produce "[[::1]]:port".
		return net.JoinHostPort(strings.Trim(explicit, "[]"), strconv.Itoa(port)), nil
	}

	if n, err := strconv.ParseUint(p, 10, 16); err != nil || n == 0 {
		return "", fmt.Errorf("invalid port in advertise address %q", explicit)
	}

	return explicit, nil
}
