package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/felixge/httpsnoop"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

type Mux struct {
	logger  zerolog.Logger
	router  *http.ServeMux
	metrics metrics
}

type metrics struct {
	inFlightRequests       prometheus.Gauge
	requestDurationVec     *prometheus.SummaryVec
	requestWrittenBytesVec *prometheus.SummaryVec
}

type MuxOpts struct {
	Logger         zerolog.Logger
	PromRegisterer interface {
		prometheus.Registerer
		prometheus.Gatherer
	}
	isReady               *readynessHandler
	changeLogLevelEnabled bool
	disconnectEnabled     bool
	pprofEnabled          bool
}

func NewMux(opts MuxOpts) *Mux {
	router := http.NewServeMux()

	router.Handle("/", defaultHandler())

	promHandler := promhttp.InstrumentMetricHandler(
		opts.PromRegisterer,
		promhttp.HandlerFor(opts.PromRegisterer,
			promhttp.HandlerOpts{
				Registry: opts.PromRegisterer,
			}),
	)

	router.Handle("/metrics", promHandler)
	isReady := &atomic.Value{}
	isReady.Store(false)
	router.Handle("/ready", opts.isReady)

	if opts.changeLogLevelEnabled {
		router.Handle("/logger", loggerHandler(opts.Logger))
	}

	// disconnect this agent from the API
	if opts.disconnectEnabled {
		router.Handle("/disconnect", http.HandlerFunc(disconnectHandler))
	}

	// Register pprof handlers
	if opts.pprofEnabled {
		router.HandleFunc("/debug/pprof/", pprof.Index)
		router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		router.HandleFunc("/debug/pprof/profile", pprof.Profile)
		router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		router.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	requestDurationVec := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "http",
		Subsystem: "requests",
		Name:      "duration_seconds",
		Help:      "request duration",
	}, []string{
		"code",
		"method",
	})

	if err := opts.PromRegisterer.Register(requestDurationVec); err != nil {
		return nil
	}

	requestWrittenBytesVec := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "http",
		Subsystem: "requests",
		Name:      "written_bytes",
		Help:      "total number of bytes written by code and method",
	}, []string{
		"code",
		"method",
	})

	if err := opts.PromRegisterer.Register(requestWrittenBytesVec); err != nil {
		return nil
	}

	inFlightRequests := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "http",
		Subsystem: "requests",
		Name:      "in_flight",
		Help:      "number of requests in flight",
	})

	if err := opts.PromRegisterer.Register(inFlightRequests); err != nil {
		return nil
	}

	return &Mux{
		logger: opts.Logger,
		router: router,
		metrics: metrics{
			inFlightRequests:       inFlightRequests,
			requestDurationVec:     requestDurationVec,
			requestWrittenBytesVec: requestWrittenBytesVec,
		},
	}
}

func (mux *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux.metrics.inFlightRequests.Inc()
	m := httpsnoop.CaptureMetrics(mux.router, w, r)
	mux.metrics.inFlightRequests.Dec()

	mux.metrics.requestDurationVec.With(prometheus.Labels{
		"code":   strconv.Itoa(m.Code),
		"method": r.Method,
	}).Observe(m.Duration.Seconds())

	mux.metrics.requestWrittenBytesVec.With(prometheus.Labels{
		"code":   strconv.Itoa(m.Code),
		"method": r.Method,
	}).Observe(float64(m.Written))

	var level zerolog.Level
	switch m.Code / 100 {
	case 1, 2, 3:
		level = zerolog.InfoLevel
	case 4:
		level = zerolog.WarnLevel
	case 5:
		level = zerolog.ErrorLevel
	default:
		level = zerolog.WarnLevel
	}

	mux.logger.WithLevel(level).
		Str("method", r.Method).
		Stringer("url", r.URL).
		Int("code", m.Code).
		Dur("duration", m.Duration).
		Int64("bytes_written", m.Written).
		Msg("handled request")
}

func defaultHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusOK)

		fmt.Fprintln(w, "hello, world!")
	})
}

// readynessHandler records whether the agent is ready to serve requests.
//
// readyness is defined by calling the method Set(true) on the handler
// at least once. Once the ready state is set, the handler never goes
// back to the unready state.
type readynessHandler int32

// NewReadynessHandler returns a new readynessHandler set to the unready
// state.
func NewReadynessHandler() *readynessHandler {
	return new(readynessHandler)
}

// Set should be called once with an argument of true to indicate that
// the agent is ready to serve requests. Calling it again, no matter the
// value of the argument, has no effect.
func (h *readynessHandler) Set(v bool) {
	if v {
		atomic.StoreInt32((*int32)(h), 1)
	}
}

// ServeHTTP implements http.Handler.
func (h *readynessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if atomic.LoadInt32((*int32)(h)) == 0 {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}

	// Signal readiness when the agent has connected once to the API.
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ready")
}

// disconnectHandler triggers a disconnection from the API.
func disconnectHandler(w http.ResponseWriter, r *http.Request) {
	// TODO(mem): this is a hack to trigger a disconnection from the
	// API, it would be cleaner to do this through a channel that
	// communicates the request to the checks updater.

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		msg := fmt.Sprintf("%s: %s", http.StatusText(http.StatusInternalServerError), err.Error())
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	// SIGUSR1 will disconnect agent from API for 1 minute
	err = p.Signal(syscall.SIGUSR1)
	if err != nil {
		msg := fmt.Sprintf("%s: %s", http.StatusText(http.StatusInternalServerError), err.Error())
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "disconnecting this agent from the API for 1 minute.")
}

func loggerHandler(logger zerolog.Logger) http.Handler {
	defaultLevel := logger.GetLevel()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		level, err := io.ReadAll(&io.LimitedReader{R: r.Body, N: 10})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Consume the rest of the request if there's anything there.
		_, _ = io.Copy(io.Discard, r.Body)

		_ = r.Body.Close()

		switch strings.TrimSpace(strings.ToLower(string(level))) {
		case "debug":
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
			logger.Warn().Stringer("level", zerolog.DebugLevel).Msg("changed log level")

		case "default":
			zerolog.SetGlobalLevel(defaultLevel)
			logger.Warn().Stringer("level", defaultLevel).Msg("changed log level")

		default:
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		}
	})
}
