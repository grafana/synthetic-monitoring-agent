package scraper

import "github.com/prometheus/client_golang/prometheus"

func NewMetrics(scrapeCounter Incrementer, errorCounter IncrementerVec) Metrics {
	return metrics{
		scrapeCounter: scrapeCounter,
		errorCounter:  errorCounter,
	}
}

type metrics struct {
	scrapeCounter Incrementer
	errorCounter  IncrementerVec
}

func (m metrics) AddScrape() {
	m.scrapeCounter.Inc()
}

func (m metrics) AddCheckError() {
	m.errorCounter.WithLabelValues("check").Inc()
}

func (m metrics) AddCollectorError() {
	m.errorCounter.WithLabelValues("collector").Inc()
}

type Incrementer interface {
	Inc()
}

type IncrementerVec interface {
	WithLabelValues(...string) Incrementer
}

type counterVecWrapper struct {
	c *prometheus.CounterVec
}

func (c *counterVecWrapper) WithLabelValues(v ...string) Incrementer {
	return c.c.WithLabelValues(v...)
}

func NewIncrementerFromCounterVec(c *prometheus.CounterVec) IncrementerVec {
	return &counterVecWrapper{c: c}
}
