package main

import (
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/fleet"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// fleetConfig groups the -fm-* flags that register the agent with Grafana
// Fleet Management over OpAMP. The environment variable names match the ones
// used by Alloy's otel-supervisor for the same purpose.
type fleetConfig struct {
	URL        string
	InstanceID string
	Token      Secret
	StateDir   string
}

func (c fleetConfig) enabled() bool {
	return c.URL != ""
}

func (c *fleetConfig) fromEnv() {
	c.URL = stringFromEnv("GCLOUD_FM_URL", c.URL)
	c.InstanceID = stringFromEnv("GCLOUD_INSTANCE_ID", c.InstanceID)
	c.Token = Secret(stringFromEnv("GCLOUD_RW_API_KEY", string(c.Token)))
}

func buildFleetAgent(cfg fleetConfig, logger zerolog.Logger, httpDebug *k6runner.HTTPDebug) (*fleet.Agent, error) {
	opts := fleet.Opts{
		Logger:     logger,
		URL:        cfg.URL,
		InstanceID: cfg.InstanceID,
		Token:      string(cfg.Token),
		StateDir:   cfg.StateDir,
	}

	// Avoid a typed nil: fleet treats a nil K6HTTPDebug as "k6 not enabled".
	if httpDebug != nil {
		opts.K6HTTPDebug = httpDebug
	}

	return fleet.New(opts)
}

// teeProbe forwards the single probe sent on in to fn and then to out, if out
// is not nil. The updater sends the probe at most once and then closes the
// channel.
func teeProbe(in <-chan *synthetic_monitoring.Probe, out chan<- *synthetic_monitoring.Probe, fn func(*synthetic_monitoring.Probe)) {
	probe, ok := <-in
	if !ok {
		if out != nil {
			close(out)
		}

		return
	}

	fn(probe)

	if out != nil {
		out <- probe
		close(out)
	}
}
