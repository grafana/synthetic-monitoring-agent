package main

import "github.com/prometheus/client_golang/prometheus"

func registerMetrics(r prometheus.Registerer) error {
	return r.Register(prometheus.NewBuildInfoCollector())
}
