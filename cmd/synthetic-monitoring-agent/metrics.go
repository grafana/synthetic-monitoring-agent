package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

func registerMetrics(r prometheus.Registerer) error {
	if err := r.Register(collectors.NewBuildInfoCollector()); err != nil {
		return err
	}

	if err := r.Register(collectors.NewGoCollector()); err != nil {
		return err
	}

	if err := r.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		return err
	}

	return nil
}
