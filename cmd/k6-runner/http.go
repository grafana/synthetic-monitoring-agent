package main

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"strconv"
	"sync/atomic"

	"github.com/felixge/httpsnoop"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

// muxOpts configures [newMux]. The dispatcher and worker both use this
// shape; only the dispatcher passes a non-nil `extra` handler with the
// /run, /dequeue, /result/{id} routes.
type muxOpts struct {
	logger     zerolog.Logger
	registerer interface {
		prometheus.Registerer
		prometheus.Gatherer
	}
	readyness    *readynessHandler
	pprofEnabled bool
	extra        http.Handler
}

type mux struct {
	router  *http.ServeMux
	logger  zerolog.Logger
	metrics httpMetrics
}

type httpMetrics struct {
	inFlight       prometheus.Gauge
	requestSeconds *prometheus.SummaryVec
	requestBytes   *prometheus.SummaryVec
}

func newMux(opts muxOpts) http.Handler {
	router := http.NewServeMux()

	router.Handle("/metrics", promhttp.InstrumentMetricHandler(
		opts.registerer,
		promhttp.HandlerFor(opts.registerer, promhttp.HandlerOpts{Registry: opts.registerer}),
	))
	router.Handle("/ready", opts.readyness)
	router.Handle("/live", opts.readyness)

	if opts.pprofEnabled {
		router.HandleFunc("/debug/pprof/", pprof.Index)
		router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		router.HandleFunc("/debug/pprof/profile", pprof.Profile)
		router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		router.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	// extra hosts the dispatcher's /run, /dequeue, /result/{id} routes; treat as catch-all so admin paths above keep
	// taking precedence and unknown paths reach the dispatcher (which 404s them).
	if opts.extra != nil {
		router.Handle("/", opts.extra)
	} else {
		router.Handle("/", defaultHandler())
	}

	durations := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "http", Subsystem: "requests", Name: "duration_seconds", Help: "Request duration in seconds.",
	}, []string{"code", "method"})
	bytesWritten := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "http", Subsystem: "requests", Name: "written_bytes", Help: "Bytes written per request.",
	}, []string{"code", "method"})
	inFlight := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "http", Subsystem: "requests", Name: "in_flight", Help: "In-flight HTTP requests.",
	})
	opts.registerer.MustRegister(durations, bytesWritten, inFlight)

	return &mux{
		router: router,
		logger: opts.logger,
		metrics: httpMetrics{
			inFlight:       inFlight,
			requestSeconds: durations,
			requestBytes:   bytesWritten,
		},
	}
}

func (m *mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.metrics.inFlight.Inc()
	captured := httpsnoop.CaptureMetrics(m.router, w, r)
	m.metrics.inFlight.Dec()

	labels := prometheus.Labels{"code": strconv.Itoa(captured.Code), "method": r.Method}
	m.metrics.requestSeconds.With(labels).Observe(captured.Duration.Seconds())
	m.metrics.requestBytes.With(labels).Observe(float64(captured.Written))

	level := zerolog.InfoLevel
	switch captured.Code / 100 {
	case 4:
		level = zerolog.WarnLevel
	case 5:
		level = zerolog.ErrorLevel
	}
	m.logger.WithLevel(level).
		Str("method", r.Method).Stringer("url", r.URL).
		Int("code", captured.Code).Dur("duration", captured.Duration).
		Int64("bytes_written", captured.Written).
		Msg("handled request")
}

func defaultHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintln(w, "k6-runner")
	})
}

// readynessHandler reports HTTP 503 until [Set](true) is called once,
// then HTTP 200. It never reverts.
type readynessHandler int32

func newReadynessHandler() *readynessHandler {
	return new(readynessHandler)
}

func (h *readynessHandler) Set(v bool) {
	if v {
		atomic.StoreInt32((*int32)(h), 1)
	}
}

func (h *readynessHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	if atomic.LoadInt32((*int32)(h)) == 0 {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "ready")
}
