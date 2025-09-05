package prober

import (
	"context"
	"fmt"
	"net/http"

	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"

	"github.com/grafana/synthetic-monitoring-agent/internal/error_types"
	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/browser"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/dns"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/grpc"
	httpProber "github.com/grafana/synthetic-monitoring-agent/internal/prober/http"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/icmp"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/multihttp"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/scripted"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/tcp"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/traceroute"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

const errUnsupportedCheckType = error_types.BasicError("unsupported check type")

type Prober interface {
	Name() string
	Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) (bool, float64)
}

func Run(ctx context.Context, p Prober, target string, registry *prometheus.Registry, logger logger.Logger) (bool, float64) {
	return p.Probe(ctx, target, registry, logger)
}

type ProberFactory interface {
	New(ctx context.Context, logger zerolog.Logger, check model.Check) (Prober, string, error)
}

type proberFactory struct {
	runner      k6runner.Runner
	probeId     int64
	features    feature.Collection
	secretStore secrets.SecretProvider
}

func NewProberFactory(runner k6runner.Runner, probeId int64, features feature.Collection, secretStore secrets.SecretProvider) ProberFactory {
	return proberFactory{
		runner:      runner,
		probeId:     probeId,
		features:    features,
		secretStore: secretStore,
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
		p, err = icmp.NewProber(check)
		target = check.Target

	case sm.CheckTypeHttp:
		reservedHeaders := f.getReservedHeaders(&check)
		p, err = httpProber.NewProber(ctx, check, logger, reservedHeaders, f.secretStore)
		target = check.Target

	case sm.CheckTypeDns:
		if f.features.IsSet(feature.ExperimentalDnsProber) {
			p, err = dns.NewExperimentalProber(check)
		} else {
			p, err = dns.NewProber(check)
		}
		target = check.Settings.Dns.Server

	case sm.CheckTypeTcp:
		p, err = tcp.NewProber(ctx, check, logger)
		target = check.Target

	case sm.CheckTypeTraceroute:
		p, err = traceroute.NewProber(check, logger)
		target = check.Target

	case sm.CheckTypeScripted:
		if f.runner != nil {
			p, err = scripted.NewProber(ctx, check, logger, f.runner, f.secretStore)
			target = check.Target
		} else {
			err = fmt.Errorf("k6 checks are not enabled")
		}

	case sm.CheckTypeBrowser:
		// TODO(mem): we possibly need to extend the validation so that
		// we know that the runner is actually able to handle browser
		// checks.
		if f.runner != nil {
			p, err = browser.NewProber(ctx, check, logger, f.runner, f.secretStore)
			target = check.Target
		} else {
			err = fmt.Errorf("k6 checks are not enabled")
		}

	case sm.CheckTypeMultiHttp:
		if f.runner != nil {
			reservedHeaders := f.getReservedHeaders(&check)
			p, err = multihttp.NewProber(ctx, check, logger, f.runner, reservedHeaders, f.secretStore)
			target = check.Target
		} else {
			err = fmt.Errorf("k6 checks are not enabled")
		}

	case sm.CheckTypeGrpc:
		p, err = grpc.NewProber(ctx, check, logger)
		target = check.Target

	default:
		return nil, "", errUnsupportedCheckType
	}

	return p, target, err
}

// Build reserved HTTP request headers for applicable checks.
func (f proberFactory) getReservedHeaders(check *model.Check) http.Header {
	reservedHeaders := http.Header{}
	if f.probeId != 0 {
		checkProbeIdentifier := fmt.Sprintf("%d-%d", check.GlobalID(), f.probeId)
		reservedHeaders.Add("x-sm-id", checkProbeIdentifier)
	}

	return reservedHeaders
}
