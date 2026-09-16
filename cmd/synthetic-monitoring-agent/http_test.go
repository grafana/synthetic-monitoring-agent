package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestReadynessHandler(t *testing.T) {
	h := NewReadynessHandler()

	// Test that the handler returns a 503 when the agent is not ready.
	var (
		w *httptest.ResponseRecorder
		r *http.Request
	)

	// A new readynessHandler should report not ready before the
	// first call to Set(true).
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/ready", nil)
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Calling Set(false) should NOT change the readyness state.
	h.Set(false)

	// Still not ready, report not ready.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/ready", nil)
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Set as ready.
	h.Set(true)

	// Now the handler should return a 200.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/ready", nil)
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "ready", w.Body.String())

	// Setting it as ready again.
	h.Set(true)

	// Response should not change.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/ready", nil)
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "ready", w.Body.String())

	// Setting it back to not ready.
	h.Set(false)

	// The handler should still return a 200 because the agent was
	// marked as ready once.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/ready", nil)
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "ready", w.Body.String())
}

func TestMuxPProf(t *testing.T) {
	newMux := func(pprofEnabled bool) *Mux {
		return NewMux(MuxOpts{
			Logger:         zerolog.Nop(),
			PromRegisterer: prometheus.NewRegistry(),
			isReady:        NewReadynessHandler(),
			pprof: pprofOpts{
				enabled:              pprofEnabled,
				blockProfileRate:     1_000_000,
				mutexProfileFraction: 100,
			},
		})
	}

	// When pprof is disabled, the /debug/pprof/ routes are not registered.
	{
		mux := newMux(false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/debug/pprof/", nil)
		mux.ServeHTTP(w, r)
		require.Equal(t, http.StatusNotFound, w.Code)
	}

	// When pprof is enabled, the index and the named profiles (including
	// block and mutex, which only produce data because the runtime rates
	// are set) are served.
	{
		mux := newMux(true)

		for _, path := range []string{
			"/debug/pprof/",
			"/debug/pprof/heap",
			"/debug/pprof/block",
			"/debug/pprof/mutex",
		} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", path, nil)
			mux.ServeHTTP(w, r)
			require.Equal(t, http.StatusOK, w.Code, "path %s", path)
		}
	}
}
