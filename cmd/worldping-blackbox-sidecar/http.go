package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Mux struct {
	router         *http.ServeMux
	requestCounter *prometheus.SummaryVec
}

type MuxOpts struct {
	Logger         *log.Logger
	PromRegisterer interface {
		prometheus.Registerer
		prometheus.Gatherer
	}
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

	requestCounter := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "http",
		Subsystem: "requests",
		Name:      "duration_seconds",
		Help:      "request duration",
	}, []string{
		"code",
		"method",
	})

	if err := opts.PromRegisterer.Register(requestCounter); err != nil {
		return nil
	}

	return &Mux{
		router:         router,
		requestCounter: requestCounter,
	}
}

type codeInterceptor struct {
	http.ResponseWriter
	code int
}

func (i *codeInterceptor) WriteHeader(statusCode int) {
	i.code = statusCode
	i.ResponseWriter.WriteHeader(statusCode)
}

func (i *codeInterceptor) Code() string {
	switch i.code {
	case 0:
		return "200"

	default:
		return strconv.Itoa(i.code)
	}
}

func (mux *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	i := &codeInterceptor{ResponseWriter: w}

	start := time.Now()
	mux.router.ServeHTTP(i, r)
	duration := time.Since(start).Seconds()

	mux.requestCounter.With(prometheus.Labels{
		"code":   i.Code(),
		"method": r.Method,
	}).Observe(duration)
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
