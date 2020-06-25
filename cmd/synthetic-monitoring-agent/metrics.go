package main

import "github.com/prometheus/client_golang/prometheus"

func registerMetrics(r prometheus.Registerer) error {
	if err := r.Register(prometheus.NewBuildInfoCollector()); err != nil {
		return err
	}

	if err := r.Register(prometheus.NewGoCollector()); err != nil {
		return err
	}

	if err := r.Register(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{})); err != nil {
		return err
	}

	return nil
}
