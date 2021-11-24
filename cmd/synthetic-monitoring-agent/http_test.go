package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
