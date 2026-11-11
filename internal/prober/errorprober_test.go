package prober

import (
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/browser"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/multihttp"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/scripted"
)

// The probers that depend on a k6 runner can tell a check that failed
// from a check that could not be run.
var (
	_ ErrorProber = scripted.Prober{}
	_ ErrorProber = browser.Prober{}
	_ ErrorProber = multihttp.Prober{}
)
