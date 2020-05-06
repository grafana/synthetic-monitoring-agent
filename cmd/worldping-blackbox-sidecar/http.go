package main

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func newRouter() *http.ServeMux {
	router := http.NewServeMux()
	router.Handle("/", defaultHandler())
	router.Handle("/metrics", promhttp.Handler())

	return router

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
