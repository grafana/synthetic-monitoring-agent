package multihttp

// Each scenario runs one multihttp check against a fake server and compares
// only what can be seen from outside the prober: overall success, the
// assertion pass and fail totals, and the requests the server saw (method and
// path in order, plus parts each request must contain). Log text, check names,
// timings and the default User-Agent are left out on purpose: they are details
// of how the check runs, not of what it does. See testdata/scenarios/README.md
// for the file format.

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	kitlog "github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scenario struct {
	Description string               `json:"description"`
	Settings    sm.MultiHttpSettings `json:"settings"`
	Routes      map[string]reply     `json:"routes"`
	Expect      expectation          `json:"expect"`
}

type reply struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Gzip    bool              `json:"gzip"`
}

type expectation struct {
	Success  bool              `json:"success"`
	Checks   checkTotals       `json:"checks"`
	Requests []expectedRequest `json:"requests"`
}

type checkTotals struct {
	Pass float64 `json:"pass"`
	Fail float64 `json:"fail"`
}

type expectedRequest struct {
	Route   string            `json:"route"`
	Headers map[string]string `json:"headers"`
	Query   map[string]string `json:"query"`
	Body    string            `json:"body"`
}

type recordedRequest struct {
	route  string
	header http.Header
	query  url.Values
	body   []byte
}

type recorder struct {
	mu   sync.Mutex
	reqs []recordedRequest
}

func TestScenarios(t *testing.T) {
	files, err := filepath.Glob("testdata/scenarios/*.json")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	k6s := testhelper.K6Paths(t)

	for _, f := range files {
		t.Run(strings.TrimSuffix(filepath.Base(f), ".json"), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(f)
			require.NoError(t, err)

			for _, k6 := range k6s {
				t.Run(filepath.Base(k6), func(t *testing.T) { runScenario(t, string(raw), k6) })
			}
		})
	}
}

func runScenario(t *testing.T, raw, k6 string) {
	listeners := []net.Listener{listen(t, "127.0.0.1:0")}
	pairs := []string{"{{base}}", "http://" + listeners[0].Addr().String()}

	alt, err := net.Listen("tcp", "127.0.0.2:0")

	switch {
	case err == nil:
		t.Cleanup(func() { _ = alt.Close() })

		listeners = append(listeners, alt)
		pairs = append(pairs, "{{alt}}", "http://"+alt.Addr().String())
	case strings.Contains(raw, "{{alt}}"):
		t.Skipf("cannot listen on 127.0.0.2, which {{alt}} needs (on macOS, lo0 has no such alias by default): %v", err)
	}

	var sc scenario

	dec := json.NewDecoder(strings.NewReader(strings.NewReplacer(pairs...).Replace(raw)))
	dec.DisallowUnknownFields()
	require.NoError(t, dec.Decode(&sc))
	require.NoError(t, sc.Settings.Validate())

	rec := &recorder{}
	for _, ln := range listeners {
		s := &httptest.Server{Listener: ln, Config: &http.Server{Handler: rec.handler(sc.Routes)}}
		s.Start()
		t.Cleanup(s.Close)
	}

	success, totals := probeOnce(t, &sc.Settings, k6)

	rec.mu.Lock()
	reqs := rec.reqs
	rec.mu.Unlock()

	gotRoutes := make([]string, 0, len(reqs))
	for _, r := range reqs {
		gotRoutes = append(gotRoutes, r.route)
	}

	wantRoutes := make([]string, 0, len(sc.Expect.Requests))
	for _, r := range sc.Expect.Requests {
		wantRoutes = append(wantRoutes, r.Route)
	}

	assert.Equal(t, sc.Expect.Success, success, "check success")
	assert.Equal(t, sc.Expect.Checks, totals, "probe_checks_total by result")
	require.Equal(t, wantRoutes, gotRoutes, "requests seen by the server, in order")

	for i, want := range sc.Expect.Requests {
		assertRequest(t, i, want, reqs[i])
	}
}

func listen(t *testing.T, addr string) net.Listener {
	ln, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	return ln
}

func (rec *recorder) handler(routes map[string]reply) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		header := r.Header.Clone()
		header.Set("Host", r.Host)
		route := r.Method + " " + r.URL.Path

		rec.mu.Lock()
		rec.reqs = append(rec.reqs, recordedRequest{route: route, header: header, query: r.URL.Query(), body: body})
		rec.mu.Unlock()

		rp, ok := routes[route]
		if !ok {
			http.Error(w, "no route", http.StatusNotFound)
			return
		}

		for name, value := range rp.Headers {
			w.Header().Set(name, value)
		}

		out := []byte(rp.Body)

		if rp.Gzip {
			var buf bytes.Buffer

			zw := gzip.NewWriter(&buf)
			_, _ = zw.Write(out)
			_ = zw.Close()
			out = buf.Bytes()

			w.Header().Set("Content-Encoding", "gzip")
		}

		w.WriteHeader(cmp.Or(rp.Status, http.StatusOK))
		_, _ = w.Write(out)
	})
}

func probeOnce(t *testing.T, settings *sm.MultiHttpSettings, k6 string) (bool, checkTotals) {
	check := model.Check{Check: sm.Check{
		Target:   settings.Entries[0].Request.Url,
		Job:      "scenario",
		Timeout:  10000,
		Settings: sm.CheckSettings{Multihttp: settings},
	}}

	ctx, cancel := testhelper.Context(context.Background(), t)
	t.Cleanup(cancel)

	runner, err := k6runner.New(k6runner.RunnerOpts{Uri: k6})
	require.NoError(t, err)

	p, err := NewProber(ctx, check, testhelper.Logger(t), runner, http.Header{}, &testhelper.NoopSecretStore{})
	require.NoError(t, err)

	var buf bytes.Buffer

	t.Cleanup(func() {
		if t.Failed() {
			t.Log("check log:\n" + buf.String())
		}
	})

	reg := prometheus.NewRegistry()
	userLogger := level.NewFilter(kitlog.NewLogfmtLogger(&buf), level.AllowInfo(), level.SquelchNoLevel(false))
	success, _ := p.Probe(ctx, check.Target, reg, userLogger, "scenario")

	mfs, err := reg.Gather()
	require.NoError(t, err)

	sums := map[string]float64{}

	for _, mf := range mfs {
		if mf.GetName() != "probe_checks_total" {
			continue
		}

		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "result" {
					sums[lp.GetValue()] += m.GetGauge().GetValue() + m.GetCounter().GetValue() + m.GetUntyped().GetValue()
				}
			}
		}
	}

	return success, checkTotals{Pass: sums["pass"], Fail: sums["fail"]}
}

func assertRequest(t *testing.T, i int, want expectedRequest, got recordedRequest) {
	for name, sub := range want.Headers {
		assert.Containsf(t, strings.Join(got.header.Values(name), ", "), sub, "request %d (%s): header %s", i, want.Route, name)
	}

	for name, sub := range want.Query {
		assert.Containsf(t, strings.Join(got.query[name], ","), sub, "request %d (%s): query %s", i, want.Route, name)
	}

	assert.Containsf(t, string(got.body), want.Body, "request %d (%s): body", i, want.Route)
}
