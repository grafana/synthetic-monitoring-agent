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
	SRCADDR          = ""
)

func ProbeTraceroute(ctx context.Context, target string, module ConfigModule, registry *prometheus.Registry, logger kitlog.Logger) bool {

	m, ch, err := mtr.NewMTR(target, SRCADDR, time.Duration(module.Traceroute.HopTimeout), INTERVAL, HOP_SLEEP, int(module.Traceroute.MaxHops), int(module.Traceroute.MaxUnknownHops), RING_BUFFER_SIZE, module.Traceroute.PtrLookup)

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
		logger.Log("Level", "info", "Destination", m.Address, "Host", hop.Target, "TTL", hop.TTL, "ElapsedTime", avgElapsedTime, "LossPercent", hop.Loss(), "Sent", hop.Sent, "TraceID", traceID)
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

	return success
}
