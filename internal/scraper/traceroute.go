package scraper

import (
	"context"
	"fmt"

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

func logHop(hop traceroute.TracerouteHop, logger kitlog.Logger) {
	addr := fmt.Sprintf("%v.%v.%v.%v", hop.Address[0], hop.Address[1], hop.Address[2], hop.Address[3])
	hostOrAddr := addr
	logger.Log("Host or address", hostOrAddr)
	if hop.Host != "" {
		hostOrAddr = hop.Host
	}
	if hop.Success {
		logger.Log(fmt.Printf("%-3d %v (%v)  %v\n", hop.TTL, hostOrAddr, addr, hop.ElapsedTime))
	} else {
		logger.Log(fmt.Printf("%-3d *\n", hop.TTL))
	}
}

func ProbeTraceroute(ctx context.Context, target string, module ConfigModule, registry *prometheus.Registry, logger kitlog.Logger) bool {
	options := traceroute.TracerouteOptions{}

	options.SetTimeoutMs(100)
	options.SetFirstHop(module.Traceroute.FirstHop)
	options.SetMaxHops(module.Traceroute.MaxHops)
	options.SetPort(module.Traceroute.Port)
	options.SetPacketSize(module.Traceroute.PacketSize)
	options.SetRetries(module.Traceroute.Retries)

	logger.Log("TARGET ***** ", target)

	c := make(chan traceroute.TracerouteHop)

	go func() {
		for {
			hop, ok := <-c
			if !ok {
				logger.Log("traceroute hop not OK")
				fmt.Println()
				return
			}
			logHop(hop, logger)
		}
	}()

	result, err := traceroute.Traceroute(target, &options)

	if err != nil {
		logger.Log(err)
		return false
	}
	logger.Log("Traceroute success", result)

	var totalHopsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_traceroute_total_hops",
		Help: "Total hops to reach a traceroute destination",
	})

	registry.MustRegister(totalHopsGauge)
	totalHopsGauge.Set(float64(len(result.Hops)))

	return true
}
