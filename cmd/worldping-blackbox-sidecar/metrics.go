package main

import "github.com/prometheus/client_golang/prometheus"

func registerMetrics() error {
	if err := prometheus.Register(prometheus.NewBuildInfoCollector()); err != nil {
		return err
	}

	return nil
}
