package scraper

import (
	"context"
	"time"

	kitlog "github.com/go-kit/kit/log"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tonobo/mtr/pkg/mtr"
)

var (
	COUNT            = 5
	TIMEOUT          = 800 * time.Millisecond
	INTERVAL         = 100 * time.Millisecond
	HOP_SLEEP        = time.Nanosecond
	MAX_HOPS         = 64
	MAX_UNKNOWN_HOPS = 30
	RING_BUFFER_SIZE = 50
	PTR_LOOKUP       = false
	srcAddr          = ""
)

func ProbeTraceroute(ctx context.Context, target string, module ConfigModule, registry *prometheus.Registry, logger kitlog.Logger, probeName string) bool {

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
	totalPacketsLost := float64(0)
	totalPacketsSent := float64(0)
	success := false
	for _, hop := range m.Statistic {
		totalPacketsLost += float64(hop.Lost)
		totalPacketsSent += float64(hop.Sent)
		avgElapsedTime := time.Duration(hop.Avg()) * time.Millisecond
		if hop.Target == m.Address {
			success = true
		}
		logger.Log("Level", "info", "Destination", m.Address, "Host", hop.Target, "TTL", hop.TTL, "ElapsedTime", avgElapsedTime, "LossPercent", hop.Loss(), "Sent", hop.Sent, "TraceID", traceID, "Probe", probeName)
	}
	var totalHopsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_traceroute_total_hops",
		Help: "Total hops to reach a traceroute destination",
	})

	var overallPacketLossGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_traceroute_packet_loss_percent",
		Help: "Overall percentage of packet loss during the traceroute",
	})

	registry.MustRegister(totalHopsGauge)
	registry.MustRegister(overallPacketLossGauge)

	totalHopsGauge.Set(float64((len(m.Statistic))))
	overallPacketLoss := totalPacketsLost / totalPacketsSent
	overallPacketLossGauge.Set(float64(overallPacketLoss))

	// 	logger.Log("Host", hostOrAddr, "ElapsedTime", hop.ElapsedTime, "TTL", hop.TTL, "Success", hop.Success, "TraceID", traceID)
	return success

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
}
