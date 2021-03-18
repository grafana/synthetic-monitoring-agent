package scraper

import (
	bbeconfig "github.com/prometheus/blackbox_exporter/config"
)

type ConfigModule struct {
	bbeconfig.Module
	Traceroute TracerouteProbe `yaml:"traceroute,omitempty"`
}
