package scraper

import (
	"context"

	"github.com/aeden/traceroute"
	kitlog "github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
)

type TracerouteProbe struct {
	FirstHop   int `yaml:"firstHop,omitempty"`
	MaxHops    int `yaml:"maxHops,omitempty"`
	PacketSize int `yaml:"packetSize,omitempty"`
	Port       int `yaml:"port,omitempty"`
	Retries    int `yaml:"retries,omitempty"`
	Timeout    int `yaml:"timeout,omitempty"`
}

func ProbeTraceroute(ctx context.Context, target string, module ConfigModule, registry *prometheus.Registry, logger kitlog.Logger) bool {
	var options *traceroute.TracerouteOptions

	result, err := traceroute.Traceroute(target, options)

	if err != nil {
		logger.Log(err)
		return false
	}

	var totalHopsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_traceroute_total_hops",
		Help: "Total hops to reach a traceroute destination",
	})

	registry.MustRegister(totalHopsGauge)
	totalHopsGauge.Set(float64(len(result.Hops)))

	return true
}
