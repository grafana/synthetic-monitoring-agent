package icmp

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	ping "github.com/prometheus-community/pro-bing"
	"github.com/prometheus/client_golang/prometheus"
)

func probeICMP(ctx context.Context, target string, module Module, registry *prometheus.Registry, logger *slog.Logger) (success bool, duration float64) {
	var (
		durationGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "probe_icmp_duration_seconds",
			Help: "Duration of icmp request by phase",
		}, []string{"phase"})

		durationMaxGauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "probe_icmp_duration_rtt_max_seconds",
			Help: "Maximum duration of round trip time phase",
		})

		durationMinGauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "probe_icmp_duration_rtt_min_seconds",
			Help: "Minimum duration of round trip time phase",
		})

		durationStddevGauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "probe_icmp_duration_rtt_stddev_seconds",
			Help: "Standard deviation of round trip time phase",
		})

		packetsSentGauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "probe_icmp_packets_sent_count",
			Help: "Number of ICMP packets sent",
		})

		packetsReceivedGauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "probe_icmp_packets_received_count",
			Help: "Number of ICMP packets received",
		})

		hopLimitGauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "probe_icmp_reply_hop_limit",
			Help: "Replied packet hop limit (TTL for ipv4)",
		})
	)

	for _, lv := range []string{"resolve", "setup", "rtt"} {
		durationGaugeVec.WithLabelValues(lv)
	}

	registry.MustRegister(durationGaugeVec)
	registry.MustRegister(durationMaxGauge)
	registry.MustRegister(durationMinGauge)
	registry.MustRegister(durationStddevGauge)
	registry.MustRegister(packetsSentGauge)
	registry.MustRegister(packetsReceivedGauge)

	dstIPAddr, lookupTime, err := chooseProtocol(ctx, module.ICMP.IPProtocol, module.ICMP.IPProtocolFallback, target, int(module.MaxResolveRetries), registry, logger)
	if err != nil {
		logger.Error("Error resolving address", "err", err)
		return false, 0
	}

	durationGaugeVec.WithLabelValues("resolve").Add(lookupTime)
	duration += lookupTime

	pinger := ping.New(dstIPAddr.String())

	pinger.SetPrivileged(module.Privileged)

	if err := pinger.Resolve(); err != nil {
		// This should never happen, the address is already resolved.
		logger.Error("Error resolving address", "err", err)
		return false, 0
	}

	pinger.Timeout = module.Timeout

	pinger.SetLogger(icmpLogger{logger})

	var (
		setupStart time.Time
		setupDone  bool
	)

	pinger.OnSetup = func() {
		if !setupDone {
			setupDuration := time.Since(setupStart).Seconds()
			durationGaugeVec.WithLabelValues("setup").Add(setupDuration)
			duration += setupDuration
			setupDone = true
		}
		logger.Info("Using source address", "srcIP", pinger.Source)
	}

	pinger.OnSend = func(pkt *ping.Packet) {
		logger.Info("Creating ICMP packet", "seq", strconv.Itoa(pkt.Seq))
		logger.Info("Waiting for reply packets")
	}

	pinger.OnRecv = func(pkt *ping.Packet) {
		if pkt.Seq == 0 && pkt.TTL >= 0 {
			registry.MustRegister(hopLimitGauge)
			hopLimitGauge.Set(float64(pkt.TTL))
		}

		logger.Info("Found matching reply packet", "seq", strconv.Itoa(pkt.Seq))
	}

	pinger.OnDuplicateRecv = func(pkt *ping.Packet) {
		logger.Info("Duplicate packet received", "seq", strconv.Itoa(pkt.Seq))
	}

	pinger.OnFinish = func(stats *ping.Statistics) {
		durationGaugeVec.WithLabelValues("rtt").Set(stats.AvgRtt.Seconds())
		duration += stats.AvgRtt.Seconds()
		durationMaxGauge.Set(stats.MaxRtt.Seconds())
		durationMinGauge.Set(stats.MinRtt.Seconds())
		durationStddevGauge.Set(stats.StdDevRtt.Seconds())
		packetsSentGauge.Set(float64(stats.PacketsSent))
		packetsReceivedGauge.Set(float64(stats.PacketsRecv))
		logger.Info("Probe finished", "packets_sent", stats.PacketsSent, "packets_received", stats.PacketsRecv)
	}

	pinger.SetDoNotFragment(module.ICMP.DontFragment)
	if module.ICMP.PayloadSize != 0 {
		pinger.Size = module.ICMP.PayloadSize
	}

	pinger.Count = int(module.PacketCount)

	pinger.Interval = 50 * time.Millisecond

	pinger.RecordRtts = false

	pinger.Source = module.ICMP.SourceIPAddress

	setupStart = time.Now()

	logger.Info("Creating socket")

	if err := pinger.RunWithContext(ctx); err != nil {
		logger.Info("failed to run ping", "err", err.Error())
		return false, 0
	}

	return pinger.PacketsSent >= int(module.ReqSuccessCount) && pinger.PacketsRecv >= int(module.ReqSuccessCount), duration
}

type icmpLogger struct {
	logger *slog.Logger
}

func (l icmpLogger) Fatalf(format string, v ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, v...))
}

func (l icmpLogger) Errorf(format string, v ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, v...))
}

func (l icmpLogger) Warnf(format string, v ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, v...))
}

func (l icmpLogger) Infof(format string, v ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, v...))
}

func (l icmpLogger) Debugf(format string, v ...interface{}) {
	l.logger.Debug(fmt.Sprintf(format, v...))
}
