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

// func logHop(hop traceroute.TracerouteHop) {
// 	addr := fmt.Sprintf("%v.%v.%v.%v", hop.Address[0], hop.Address[1], hop.Address[2], hop.Address[3])
// 	hostOrAddr := addr
// 	if hop.Host != "" {
// 		hostOrAddr = hop.Host
// 	}
// 	if hop.Success {
// 		fmt.Printf("%-3d %v (%v)  %v\n", hop.TTL, hostOrAddr, addr, hop.ElapsedTime)
// 	} else {
// 		fmt.Printf("%-3d *\n", hop.TTL)
// 	}
// }

func ProbeTraceroute(ctx context.Context, target string, module ConfigModule, registry *prometheus.Registry, logger kitlog.Logger) bool {
	options := traceroute.TracerouteOptions{}

	options.SetTimeoutMs(module.Traceroute.Timeout * 1000)
	options.SetFirstHop(module.Traceroute.FirstHop)
	options.SetMaxHops(module.Traceroute.MaxHops)
	options.SetPort(module.Traceroute.Port)
	options.SetPacketSize(module.Traceroute.PacketSize)
	options.SetRetries(module.Traceroute.Retries)

	// c := make(chan traceroute.TracerouteHop, 0)

	// go func() {
	// 	for {
	// 		hop, ok := <-c
	// 		if !ok {
	// 			fmt.Println()
	// 			return
	// 		}
	// 		logHop(hop)
	// 	}
	// }()

	result, err := traceroute.Traceroute(target, &options)

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
