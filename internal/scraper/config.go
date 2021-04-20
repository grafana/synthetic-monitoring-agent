package scraper

import (
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	bbeconfig "github.com/prometheus/blackbox_exporter/config"
)

type ConfigModule struct {
	bbeconfig.Module
	Traceroute synthetic_monitoring.TracerouteSettings `yaml:"traceroute,omitempty"`
}
