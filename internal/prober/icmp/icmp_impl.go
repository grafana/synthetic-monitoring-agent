package icmp

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"       //nolint:staticcheck // TODO(mem): replace in BBE
	"github.com/go-kit/kit/log/level" //nolint:staticcheck // TODO(mem): replace in BBE
	"github.com/go-ping/ping"
	"github.com/prometheus/client_golang/prometheus"
)

func probeICMP(ctx context.Context, target string, module Module, registry *prometheus.Registry, logger log.Logger) (success bool) {
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

	dstIPAddr, lookupTime, err := chooseProtocol(ctx, module.ICMP.IPProtocol, module.ICMP.IPProtocolFallback, target, registry, logger)
	if err != nil {
		_ = level.Warn(logger).Log("msg", "Error resolving address", "err", err)
		return false
	}

	durationGaugeVec.WithLabelValues("resolve").Add(lookupTime)

	pinger := ping.New(dstIPAddr.String())

	pinger.SetPrivileged(module.Privileged)

	if err := pinger.Resolve(); err != nil {
		// This should never happe, the address is already resolved.
		_ = level.Error(logger).Log("msg", "Error resolving address", "err", err)
		return false
	}

	pinger.Timeout = module.Timeout

	pinger.SetLogger(icmpLogger{logger})

	var (
		setupStart time.Time
		setupDone  bool
	)

	pinger.OnSetup = func() {
		if !setupDone {
			durationGaugeVec.WithLabelValues("setup").Add(time.Since(setupStart).Seconds())
			setupDone = true
		}
		_ = level.Info(logger).Log("msg", "Using source address", "srcIP", pinger.Source)
	}

	pinger.OnSend = func(pkt *ping.Packet) {
		_ = level.Info(logger).Log("msg", "Creating ICMP packet", "seq", strconv.Itoa(pkt.Seq))
		_ = level.Info(logger).Log("msg", "Waiting for reply packets")
	}

	pinger.OnRecv = func(pkt *ping.Packet) {
		if pkt.Seq == 0 && pkt.Ttl >= 0 {
			registry.MustRegister(hopLimitGauge)
			hopLimitGauge.Set(float64(pkt.Ttl))
		}

		_ = level.Info(logger).Log("msg", "Found matching reply packet", "seq", strconv.Itoa(pkt.Seq))
	}

	pinger.OnDuplicateRecv = func(pkt *ping.Packet) {
		_ = level.Info(logger).Log("msg", "Duplicate packet received", "seq", strconv.Itoa(pkt.Seq))
	}

	pinger.OnFinish = func(stats *ping.Statistics) {
		durationGaugeVec.WithLabelValues("rtt").Set(stats.AvgRtt.Seconds())
		durationMaxGauge.Set(stats.MaxRtt.Seconds())
		durationMinGauge.Set(stats.MinRtt.Seconds())
		durationStddevGauge.Set(stats.StdDevRtt.Seconds())
		packetsSentGauge.Set(float64(stats.PacketsSent))
		packetsReceivedGauge.Set(float64(stats.PacketsRecv))
		_ = level.Info(logger).Log("msg", "Probe finished", "packets_sent", stats.PacketsSent, "packets_received", stats.PacketsRecv)
	}

	// TODO: module.ICMP.DontFragment

	if module.ICMP.DontFragment {
		_ = level.Warn(logger).Log("msg", "ignoring DontFragment option")
	}

	if module.ICMP.PayloadSize != 0 {
		pinger.Size = module.ICMP.PayloadSize
	}

	pinger.Count = int(module.PacketCount)

	pinger.Interval = 50 * time.Millisecond

	pinger.RecordRtts = false

	pinger.Source = module.ICMP.SourceIPAddress

	setupStart = time.Now()

	_ = level.Info(logger).Log("msg", "Creating socket")

	if err := pinger.Run(); err != nil {
		_ = level.Info(logger).Log("msg", "failed to run ping", "err", err.Error())
		return false
	}

	return pinger.Count == pinger.PacketsSent && pinger.PacketsRecv == pinger.PacketsSent
}

type icmpLogger struct {
	logger log.Logger
}

func (l icmpLogger) Fatalf(format string, v ...interface{}) {
	_ = level.Error(l.logger).Log("msg", fmt.Sprintf(format, v...))
}

func (l icmpLogger) Errorf(format string, v ...interface{}) {
	_ = level.Error(l.logger).Log("msg", fmt.Sprintf(format, v...))
}

func (l icmpLogger) Warnf(format string, v ...interface{}) {
	_ = level.Warn(l.logger).Log("msg", fmt.Sprintf(format, v...))
}

func (l icmpLogger) Infof(format string, v ...interface{}) {
	_ = level.Info(l.logger).Log("msg", fmt.Sprintf(format, v...))
}

func (l icmpLogger) Debugf(format string, v ...interface{}) {
	_ = level.Debug(l.logger).Log("msg", fmt.Sprintf(format, v...))
}
