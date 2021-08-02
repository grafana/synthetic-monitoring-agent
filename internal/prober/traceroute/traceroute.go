package traceroute

import (
	"context"
	"errors"
	"hash/fnv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/tonobo/mtr/pkg/mtr"
)

var errUnsupportedCheck = errors.New("unsupported check")

type Module struct {
	count          int
	timeout        time.Duration
	interval       time.Duration
	hopSleep       time.Duration
	maxHops        int
	maxUnknownHops int
	ptrLookup      bool
	srcAddr        string
	ringBufferSize int
}

type Prober struct {
	config Module
	logger zerolog.Logger
}

func NewProber(check sm.Check, logger zerolog.Logger) (Prober, error) {
	if check.Settings.Traceroute == nil {
		return Prober{}, errUnsupportedCheck
	}

	c := settingsToModule(check.Settings.Traceroute)

	return Prober{
		config: c,
		logger: logger,
	}, nil
}

func (p Prober) Name() string {
	return "traceroute"
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	m, ch, err := mtr.NewMTR(target, p.config.srcAddr, p.config.timeout, p.config.interval, p.config.hopSleep, p.config.maxHops, p.config.maxUnknownHops, p.config.ringBufferSize, p.config.ptrLookup)

	if err != nil {
		logErr := logger.Log(err)
		if logErr != nil {
			p.logger.Error().Err(logErr)
			return false
		}
		return false
	}

	go func(ch <-chan struct{}) {
		for {
			_, isOpen := <-ch
			if !isOpen {
				return
			}
		}
	}(ch)
	m.Run(p.config.count)

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
		err := logger.Log("Level", "info", "Destination", m.Address, "Hosts", targets, "TTL", hop.TTL, "ElapsedTime", avgElapsedTime, "LossPercent", hop.Loss(), "Sent", hop.Sent, "TraceID", traceID)
		if err != nil {
			p.logger.Error().Err(err)
			continue
		}
	}

	traceHash := fnv.New32()
	_, err = traceHash.Write([]byte(hosts))

	if err != nil {
		p.logger.Error().Err(err)
		return false
	}

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

	// It shouldn't be possible for these registrations to fail
	registry.MustRegister(traceHashGauge)
	registry.MustRegister(totalHopsGauge)
	registry.MustRegister(overallPacketLossGauge)

	traceHashGauge.Set(float64(traceHash.Sum32()))
	totalHopsGauge.Set(float64((len(m.Statistic))))
	overallPacketLoss := totalPacketsLost / totalPacketsSent
	overallPacketLossGauge.Set(overallPacketLoss)

	return success
}

func settingsToModule(settings *sm.TracerouteSettings) Module {
	m := Module{
		count:          5,
		timeout:        800 * time.Millisecond,
		interval:       100 * time.Millisecond,
		hopSleep:       time.Nanosecond,
		maxHops:        64,
		maxUnknownHops: 15,
		ptrLookup:      false,
		ringBufferSize: 50,
		srcAddr:        "",
	}

	if settings.MaxHops > 0 {
		m.maxUnknownHops = int(settings.MaxHops)
	}

	if settings.MaxUnknownHops > 1 {
		m.maxUnknownHops = int(settings.MaxUnknownHops)
	}

	if settings.HopTimeout > 0 {
		m.timeout = time.Duration(settings.HopTimeout)
	}

	return m
}
