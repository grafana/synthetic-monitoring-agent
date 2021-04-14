package scraper

import (
	"context"
	"time"

	kitlog "github.com/go-kit/kit/log"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tonobo/mtr/pkg/mtr"
)

type TracerouteProbe struct {
	FirstHop   int `yaml:"firstHop,omitempty"`
	MaxHops    int `yaml:"maxHops,omitempty"`
	PacketSize int `yaml:"packetSize,omitempty"`
	Port       int `yaml:"port,omitempty"`
	Retries    int `yaml:"retries,omitempty"`
	Timeout    int `yaml:"timeout,omitempty"`
}

var (
	COUNT            = 5
	TIMEOUT          = 800 * time.Millisecond
	INTERVAL         = 100 * time.Millisecond
	HOP_SLEEP        = time.Nanosecond
	MAX_HOPS         = 64
	MAX_UNKNOWN_HOPS = 10
	RING_BUFFER_SIZE = 50
	PTR_LOOKUP       = false
	srcAddr          = ""
)

func ProbeTraceroute(ctx context.Context, target string, module ConfigModule, registry *prometheus.Registry, logger kitlog.Logger) bool {

	m, ch, err := mtr.NewMTR(target, srcAddr, TIMEOUT, INTERVAL, HOP_SLEEP, MAX_HOPS, MAX_UNKNOWN_HOPS, RING_BUFFER_SIZE, PTR_LOOKUP)

	if err != nil {
		logger.Log(err)
		return false
	}

	go func(ch chan struct{}) {
		for {
			<-ch
		}
	}(ch)
	m.Run(ch, COUNT)

	traceID := uuid.New()
	for _, hop := range m.Statistic {
		logger.Log("Level", "info", "Destination", m.Address, "Host", hop.Target, "TTL", hop.TTL, "AvgMs", hop.Avg(), "LossPercent", hop.Loss(), "Sent", hop.Sent, "TraceID", traceID)
	}
	var totalHopsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_traceroute_total_hops",
		Help: "Total hops to reach a traceroute destination",
	})

	registry.MustRegister(totalHopsGauge)
	totalHopsGauge.Set(float64((len(m.Statistic))))

	// 	logger.Log("Host", hostOrAddr, "ElapsedTime", hop.ElapsedTime, "TTL", hop.TTL, "Success", hop.Success, "TraceID", traceID)
	return true

	// m, ch, err := mtr.NewMTR(args[0], srcAddr, TIMEOUT, INTERVAL, HOP_SLEEP,
	// 	MAX_HOPS, MAX_UNKNOWN_HOPS, RING_BUFFER_SIZE, PTR_LOOKUP)

	// timeout := int(module.Module.Timeout)

	// if timeout == 0 {
	// 	timeout = 30
	// }

	// options.SetTimeoutMs(timeout)
	// options.SetFirstHop(module.Traceroute.FirstHop)
	// options.SetMaxHops(module.Traceroute.MaxHops)
	// options.SetPort(module.Traceroute.Port)
	// options.SetPacketSize(module.Traceroute.PacketSize)
	// options.SetRetries(module.Traceroute.Retries)

	// result, err := traceroute.Traceroute(target, &options)

	// if err != nil {
	// 	logger.Log(err)
	// 	return false
	// }

	// traceID := uuid.New()

	// for _, hop := range result.Hops {
	// 	// addr := fmt.Sprintf("%v.%v.%v.%v", hop.Address[0], hop.Address[1], hop.Address[2], hop.Address[3])
	// 	hostOrAddr := hop.AddressString()
	// 	// otherHost := hop.HostOrAddressString()
	// 	if hop.Host != "" {
	// 		hostOrAddr = hop.Host
	// 	}
	// 	logger.Log("Host", hostOrAddr, "ElapsedTime", hop.ElapsedTime, "TTL", hop.TTL, "Success", hop.Success, "TraceID", traceID)
	// }

	return true
}
