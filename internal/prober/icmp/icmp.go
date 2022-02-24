package icmp

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	"github.com/prometheus/client_golang/prometheus"
)

var errUnsupportedCheck = errors.New("unsupported check")

type Module struct {
	Prober      string
	Timeout     time.Duration
	PacketCount int64
	ICMP        config.ICMPProbe
	Privileged  bool
}

type Prober struct {
	config Module
}

func NewProber(check sm.Check) (Prober, error) {
	var p Prober

	if check.Settings.Ping == nil {
		return p, errUnsupportedCheck
	}

	p.config = settingsToModule(check.Settings.Ping)
	p.config.Timeout = time.Duration(check.Timeout) * time.Millisecond
	p.config.Privileged = isPrivilegedRequired()

	return p, nil
}

func (p Prober) Name() string {
	return "ping"
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	return probeICMP(ctx, target, p.config, registry, logger)
}

func settingsToModule(settings *sm.PingSettings) Module {
	var m Module

	m.Prober = sm.CheckTypePing.String()

	m.ICMP.IPProtocol, m.ICMP.IPProtocolFallback = settings.IpVersion.ToIpProtocol()

	m.ICMP.SourceIPAddress = settings.SourceIpAddress

	m.ICMP.PayloadSize = int(settings.PayloadSize)

	m.ICMP.DontFragment = settings.DontFragment

	if settings.PacketCount <= 1 {
		m.PacketCount = 1
	} else {
		m.PacketCount = settings.PacketCount
	}

	return m
}

var (
	privilegedRequired   bool
	privilegedCheckDone  bool
	privilegedCheckMutex sync.Mutex
)

func isPrivilegedRequired() bool {
	privilegedCheckMutex.Lock()
	defer privilegedCheckMutex.Unlock()

	if privilegedCheckDone {
		return privilegedRequired
	}

	var (
		ctx      = context.Background()
		target   = "127.0.0.1"
		registry = prometheus.NewRegistry()
		logger   = log.NewNopLogger()
		config   = Module{
			Prober:      "test-unprivileged",
			Timeout:     1 * time.Second,
			PacketCount: 1,
			Privileged:  false,
			ICMP: config.ICMPProbe{
				IPProtocol: "ip4",
			},
		}
	)

	success := probeICMP(ctx, target, config, registry, logger)

	privilegedRequired = !success
	privilegedCheckDone = true

	return privilegedRequired
}
