package prober

import (
	"context"
	"fmt"
	"log"
	"net"
	"syscall"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/dns"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/http"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/icmp"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/tcp"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/traceroute"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

type Prober interface {
	Name() string
	Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool
}

func Run(ctx context.Context, p Prober, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	return p.Probe(ctx, target, registry, logger)
}

func NewFromCheck(ctx context.Context, logger zerolog.Logger, check sm.Check) (Prober, string, error) {
	var (
		p      Prober
		target string
		err    error
	)

	switch checkType := check.Type(); checkType {
	case sm.CheckTypePing:
		p, err = icmp.NewProber(check)
		target = check.Target

	case sm.CheckTypeHttp:
		p, err = http.NewProber(ctx, check, logger)
		target = check.Target

	case sm.CheckTypeDns:
		p, err = dns.NewProber(check)
		target = check.Settings.Dns.Server

	case sm.CheckTypeTcp:
		p, err = tcp.NewProber(ctx, check, logger)
		target = check.Target

	case sm.CheckTypeTraceroute:
		p, err = traceroute.NewProber(check, logger)
		target = check.Target

	default:
		return nil, "", fmt.Errorf("unsupported change")
	}

	return p, target, err
}

func SetupBlockedCidrs(cidrs []string) error {
	ranges := []net.IPNet{}

	x := func() *net.Dialer {
		return &net.Dialer{
			Control: func(network, address string, c syscall.RawConn) error {
				addr, _, err := net.SplitHostPort(address)
				if err != nil {
					return err // TODO: is this correct? Or should we continue?
				}
				ip := net.ParseIP(addr)
				if ip == nil {
					return fmt.Errorf("%s is not a valid IP address", addr)
				}
				for _, r := range ranges {
					if r.Contains(ip) {
						return fmt.Errorf("address blocked")
					}
				}
				return nil
			},
		}
	}
	log.Println(x)
	// bbeprober.Dialer = x
	return nil
}
