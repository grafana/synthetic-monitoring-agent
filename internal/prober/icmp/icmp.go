package icmp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	"github.com/prometheus/client_golang/prometheus"
)

var errUnsupportedCheck = errors.New("unsupported check")

type Module struct {
	Prober            string
	Timeout           time.Duration
	PacketCount       int64
	PacketWaitCount   int64
	ReqSuccessCount   int64
	MaxResolveRetries int64
	ICMP              config.ICMPProbe
	Privileged        bool
}

type Prober struct {
	config Module
}

func NewProber(check model.Check) (Prober, error) {
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

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger) (bool, float64) {
	l.Log("config", fmt.Sprintf("%#v", p.config))
	return probeICMP(ctx, target, p.config, registry, l)
}

func settingsToModule(settings *sm.PingSettings) Module {
	var m Module

	m.Prober = sm.CheckTypePing.String()

	m.ICMP.IPProtocol, m.ICMP.IPProtocolFallback = settings.IpVersion.ToIpProtocol()

	m.ICMP.SourceIPAddress = settings.SourceIpAddress

	m.ICMP.PayloadSize = int(settings.PayloadSize)

	m.ICMP.DontFragment = settings.DontFragment

	m.PacketCount = settings.PacketCount
	m.PacketWaitCount = settings.WaitCount
	m.ReqSuccessCount = settings.SuccessCount

	if m.PacketCount == 0 {
		m.PacketCount = 3 // Send out 3 by default.
	}

	if m.PacketWaitCount == 0 {
		m.PacketWaitCount = 1 // m.PacketCount // Wait for all of them.
	}

	if m.ReqSuccessCount == 0 {
		m.ReqSuccessCount = 1 // Receiving at least 1 is considered success.
	}

	m.MaxResolveRetries = 3 // TODO(mem): add a setting for this

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
			Prober:          "test-unprivileged",
			Timeout:         1 * time.Second,
			PacketCount:     1,
			PacketWaitCount: 1,
			ReqSuccessCount: 1,
			Privileged:      false,
			ICMP: config.ICMPProbe{
				IPProtocol: "ip4",
			},
		}
	)

	success, _ := probeICMP(ctx, target, config, registry, logger)

	privilegedRequired = !success
	privilegedCheckDone = true

	return privilegedRequired
}
