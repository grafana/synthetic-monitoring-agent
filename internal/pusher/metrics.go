package pusher

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	LabelValueMetrics        = "metrics"
	LabelValueLogs           = "logs"
	LabelValueClient         = "client"
	LabelValueRetryExhausted = "retry_exhausted"
	LabelValueTenant         = "tenant"
)

// Metrics contains the prometheus Metrics for a publisher.
type Metrics struct {
	PushCounter    *prometheus.CounterVec
	ErrorCounter   *prometheus.CounterVec
	BytesOut       *prometheus.CounterVec
	FailedCounter  *prometheus.CounterVec
	RetriesCounter *prometheus.CounterVec

	// For experimental publisher only
	DroppedCounter  *prometheus.CounterVec
	ResponseCounter *prometheus.CounterVec
}

var (
	labelsWithType       = []string{"regionID", "tenantID", "type"}
	labelsWithTypeStatus = []string{"regionID", "tenantID", "type", "status"}
)

// NewMetrics returns a new set of publisher metrics registered in the given registerer.
func NewMetrics(promRegisterer prometheus.Registerer) (m Metrics) {
	m.PushCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "publisher",
			Name:      "push_total",
			Help:      "Total number of push events by type.",
		},
		labelsWithType)

	promRegisterer.MustRegister(m.PushCounter)

	m.ErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "publisher",
			Name:      "push_errors_total",
			Help:      "Total number of push errors by type and status.",
		},
		labelsWithTypeStatus)

	promRegisterer.MustRegister(m.ErrorCounter)

	m.FailedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "publisher",
			Name:      "push_failed_total",
			Help:      "Total number of push failures by type.",
		},
		labelsWithType)

	promRegisterer.MustRegister(m.FailedCounter)

	m.BytesOut = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "publisher",
			Name:      "push_bytes",
			Help:      "Total number of bytes pushed by type.",
		},
		labelsWithType)

	promRegisterer.MustRegister(m.BytesOut)

	m.RetriesCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "publisher",
			Name:      "retries_total",
			Help:      "Total number of retries performed by type.",
		},
		labelsWithType)

	promRegisterer.MustRegister(m.RetriesCounter)

	m.DroppedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "publisher",
			Name:      "drop_total",
			Help:      "Total number of results dropped by type.",
		},
		labelsWithType)

	promRegisterer.MustRegister(m.DroppedCounter)

	m.ResponseCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "publisher",
			Name:      "responses_total",
			Help:      "Total number of responses received by type and status code.",
		},
		labelsWithTypeStatus)

	promRegisterer.MustRegister(m.ResponseCounter)

	return m
}

// WithTenant returns a new set of Metrics with the local and region ID labels
// already included.
func (m Metrics) WithTenant(localID int64, regionID int) Metrics {
	labels := prometheus.Labels{
		"regionID": strconv.FormatInt(int64(regionID), 10),
		"tenantID": strconv.FormatInt(localID, 10),
	}
	return Metrics{
		PushCounter:     m.PushCounter.MustCurryWith(labels),
		ErrorCounter:    m.ErrorCounter.MustCurryWith(labels),
		BytesOut:        m.BytesOut.MustCurryWith(labels),
		FailedCounter:   m.FailedCounter.MustCurryWith(labels),
		RetriesCounter:  m.RetriesCounter.MustCurryWith(labels),
		DroppedCounter:  m.DroppedCounter.MustCurryWith(labels),
		ResponseCounter: m.ResponseCounter.MustCurryWith(labels),
	}
}

// WithType returns a new set of Metrics with the given type label.
func (m Metrics) WithType(t string) Metrics {
	var typeLabels = prometheus.Labels{
		"type": t,
	}

	return Metrics{
		PushCounter:     m.PushCounter.MustCurryWith(typeLabels),
		ErrorCounter:    m.ErrorCounter.MustCurryWith(typeLabels),
		BytesOut:        m.BytesOut.MustCurryWith(typeLabels),
		FailedCounter:   m.FailedCounter, // type in failed counter servers a different purpose.
		RetriesCounter:  m.RetriesCounter.MustCurryWith(typeLabels),
		DroppedCounter:  m.DroppedCounter.MustCurryWith(typeLabels),
		ResponseCounter: m.ResponseCounter.MustCurryWith(typeLabels),
	}
}
