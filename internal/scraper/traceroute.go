package scraper

import (
	"context"
	"fmt"

	"github.com/aeden/traceroute"
	kitlog "github.com/go-kit/kit/log"
	"github.com/google/uuid"
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
	options := traceroute.TracerouteOptions{}

	timeout := int(module.Module.Timeout)

	if timeout == 0 {
		timeout = 30000
	}

	options.SetTimeoutMs(timeout)
	options.SetFirstHop(module.Traceroute.FirstHop)
	options.SetMaxHops(module.Traceroute.MaxHops)
	options.SetPort(module.Traceroute.Port)
	options.SetPacketSize(module.Traceroute.PacketSize)
	options.SetRetries(module.Traceroute.Retries)

	result, err := traceroute.Traceroute(target, &options)

	if err != nil {
		logger.Log(err)
		return false
	}

	traceID := uuid.New()
	var totalHopsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_traceroute_total_hops",
		Help: "Total hops to reach a traceroute destination",
	})

	registry.MustRegister(totalHopsGauge)
	totalHopsGauge.Set(float64(len(result.Hops)))
	for _, hop := range result.Hops {
		addr := fmt.Sprintf("%v.%v.%v.%v", hop.Address[0], hop.Address[1], hop.Address[2], hop.Address[3])
		hostOrAddr := addr
		if hop.Host != "" {
			hostOrAddr = hop.Host
		}
		logger.Log("Host", hostOrAddr, "ElapsedTime", hop.ElapsedTime, "TTL", hop.TTL, "Success", hop.Success, "TraceID", traceID)
	}

	return true
}
