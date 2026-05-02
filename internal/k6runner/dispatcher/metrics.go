package dispatcher

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds the prometheus collectors exposed by the dispatcher.
// Phase 1 ships the small set needed to validate the design (queue
// depth, failure causes, mapping errors). The full observability set
// from the spec lands in phase 3.
type Metrics struct {
	// QueueDepth tracks how many jobs are waiting in each tier's queue.
	// Updated lazily on enqueue / dequeue / drain transitions.
	QueueDepth *prometheus.GaugeVec
	// FailureCauses counts dispatcher-side outcomes by tier and error
	// code (the values declared in [github.com/grafana/synthetic-monitoring-agent/internal/k6runner].
	// ErrorCode*).
	FailureCauses *prometheus.CounterVec
	// MappingErrors mirrors [tiermap.Metrics.MappingErrors] but is
	// owned by the dispatcher because the dispatcher is the only
	// component that can detect the unknown_tier case (the tiermap
	// package does not know which tiers are deployed).
	MappingErrors *prometheus.CounterVec
}

// NewMetrics registers the dispatcher's prometheus collectors on r.
func NewMetrics(r prometheus.Registerer) *Metrics {
	m := &Metrics{
		QueueDepth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "k6_runner",
				Subsystem: "dispatcher",
				Name:      "queue_depth",
				Help:      "Number of jobs currently waiting in each tier's queue.",
			},
			[]string{"tier"},
		),
		FailureCauses: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "k6_runner",
				Subsystem: "dispatcher",
				Name:      "failure_cause_total",
				Help:      "Number of /run requests that returned a failure, labelled by tier and error code.",
			},
			[]string{"tier", "cause"},
		),
		MappingErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "k6_runner",
				Subsystem: "dispatcher",
				Name:      "tier_mapping_errors_total",
				Help: "Mapping problems detected at dispatch time. cause=unknown_tier means the tenant's mapping " +
					"resolved to a tier that has no deployed worker pool.",
			},
			[]string{"cause"},
		),
	}
	r.MustRegister(m.QueueDepth, m.FailureCauses, m.MappingErrors)
	return m
}
