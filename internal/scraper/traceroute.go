package scraper

import (
	"context"
	"hash/fnv"
	"strings"
	"time"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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
	MAX_UNKNOWN_HOPS = 15
	RING_BUFFER_SIZE = 50
	PTR_LOOKUP       = false
	SRCADDR          = ""
)

func ProbeTraceroute(ctx context.Context, target string, module ConfigModule, registry *prometheus.Registry, logger kitlog.Logger) bool {
	var maxUnknownHops = int(module.Traceroute.MaxUnknownHops)
	if maxUnknownHops < 1 {
		maxUnknownHops = MAX_UNKNOWN_HOPS
	}

	var hopTimeout = time.Duration(module.Traceroute.HopTimeout)
	if hopTimeout < 1 {
		hopTimeout = TIMEOUT
	}

	m, ch, err := mtr.NewMTR(target, SRCADDR, hopTimeout, INTERVAL, HOP_SLEEP, int(module.Traceroute.MaxHops), maxUnknownHops, RING_BUFFER_SIZE, module.Traceroute.PtrLookup)

	if err != nil {
		logErr := level.Error(logger).Log(err)
		if logErr != nil {
			return false
		}
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
	hosts := ""
	for _, hop := range m.Statistic {
		totalPacketsLost += float64(hop.Lost)
		totalPacketsSent += float64(hop.Sent)
		avgElapsedTime := time.Duration(hop.Avg()) * time.Millisecond
		if hop.Dest.IP.String() == m.Address {
			success = true
		}
		targets := strings.Join(hop.Targets, ",")
		hosts += targets
		err := level.Info(logger).Log("Level", "info", "Destination", m.Address, "Hosts", targets, "TTL", hop.TTL, "ElapsedTime", avgElapsedTime, "LossPercent", hop.Loss(), "Sent", hop.Sent, "TraceID", traceID)
		if err != nil {
			continue
		}
	}

	traceHash := fnv.New32()
	traceHash.Write([]byte(hosts))

	var traceHashGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_traceroute_route_hash",
		Help: "Hash of all the hosts in a traceroute path. Used to determine route volatility.",
	})

	var totalHopsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_traceroute_total_hops",
		Help: "Total hops to reach a traceroute destination",
	})

	var overallPacketLossGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_traceroute_packet_loss_percent",
		Help: "Overall percentage of packet loss during the traceroute",
	})

	registry.MustRegister(traceHashGauge)
	registry.MustRegister(totalHopsGauge)
	registry.MustRegister(overallPacketLossGauge)

	traceHashGauge.Set(float64(traceHash.Sum32()))
	totalHopsGauge.Set(float64((len(m.Statistic))))
	overallPacketLoss := totalPacketsLost / totalPacketsSent
	overallPacketLossGauge.Set(overallPacketLoss)

	return success
}
