package main

import "github.com/grafana/synthetic-monitoring-agent/internal/version"

func main() {
	println(version.Short())
}
