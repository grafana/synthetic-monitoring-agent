package prober

import (
	"context"
	"fmt"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/dns"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/http"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/icmp"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/k6"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/multihttp"
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

type ProberFactory interface {
	New(ctx context.Context, logger zerolog.Logger, check model.Check) (Prober, string, error)
}

type proberFactory struct {
	runner k6runner.Runner
}

func NewProberFactory(runner k6runner.Runner) ProberFactory {
	return proberFactory{
		runner: runner,
	}
}

func (f proberFactory) New(ctx context.Context, logger zerolog.Logger, check model.Check) (Prober, string, error) {
	var (
		p      Prober
		target string
		err    error
	)

	switch checkType := check.Type(); checkType {
	case sm.CheckTypePing:
		p, err = icmp.NewProber(check.Check)
		target = check.Target

	case sm.CheckTypeHttp:
		p, err = http.NewProber(ctx, check.Check, logger)
		target = check.Target

	case sm.CheckTypeDns:
		p, err = dns.NewProber(check.Check)
		target = check.Settings.Dns.Server

	case sm.CheckTypeTcp:
		p, err = tcp.NewProber(ctx, check.Check, logger)
		target = check.Target

	case sm.CheckTypeTraceroute:
		p, err = traceroute.NewProber(check.Check, logger)
		target = check.Target

	case sm.CheckTypeK6:
		if f.runner != nil {
			p, err = k6.NewProber(ctx, check.Check, logger, f.runner)
			target = check.Target
		} else {
			err = fmt.Errorf("k6 checks are not enabled")
		}

	case sm.CheckTypeMultiHttp:
		if f.runner != nil {
			p, err = multihttp.NewProber(ctx, check.Check, logger, f.runner)
			target = check.Target
		} else {
			err = fmt.Errorf("k6 checks are not enabled")
		}

	default:
		return nil, "", fmt.Errorf("unsupported check type")
	}

	return p, target, err
}
