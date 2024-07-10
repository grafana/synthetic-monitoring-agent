package scraper

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logfmt/logfmt"
	logproto "github.com/grafana/loki/pkg/push"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/browser"
	dnsProber "github.com/grafana/synthetic-monitoring-agent/internal/prober/dns"
	grpcProber "github.com/grafana/synthetic-monitoring-agent/internal/prober/grpc"
	httpProber "github.com/grafana/synthetic-monitoring-agent/internal/prober/http"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/icmp"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/multihttp"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/scripted"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/tcp"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/traceroute"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/telemetry"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/miekg/dns"
	"github.com/mmcloughlin/geohash"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
	"kernel.org/pub/linux/libs/security/libcap/cap"
)

var updateGolden = flag.Bool("update-golden", false, "update golden files")

// TestValidateMetrics verify that the exposed metrics have not changed.
//
// It does this by setting up local targets (HTTP server, DNS server,
// TCP server) that the BBE probers can run against. The resulting
// metrics are then compared (only names and labels, not values) against
// known outputs. If metrics are added or removed, this test will fail.
//
// The golden files can be updated by running:
//
// go test -v -race -run TestValidateMetrics ./internal/scraper/
func TestValidateMetrics(t *testing.T) {
	testcases := map[string]struct {
		setup func(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func())
	}{
		"ping": {
			setup: setupPingProbe,
		},
		"http": {
			setup: setupHTTPProbe,
		},
		"http_ssl": {
			setup: setupHTTPSSLProbe,
		},
		"dns": {
			setup: setupDNSProbe,
		},
		"tcp": {
			setup: setupTCPProbe,
		},
		"tcp_ssl": {
			setup: setupTCPSSLProbe,
		},
		"traceroute": {
			setup: setupTracerouteProbe,
		},
		"scripted": {
			setup: setupScriptedProbe,
		},
		"multihttp": {
			setup: setupMultiHTTPProbe,
		},
		"grpc": {
			setup: setupGRPCProbe,
		},
		"grpc_ssl": {
			setup: setupGRPCSSLProbe,
		},
		"browser": {
			setup: setupBrowserProbe,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			verifyProberMetrics(t, name, testcase.setup, false)
		})
		t.Run(name+"_basic", func(t *testing.T) {
			verifyProberMetrics(t, name+"_basic", testcase.setup, true)
		})
	}
}

// verifyProberMetrics runs the specified prober against the server (if
// any) started by the setup function. The resup function provides the
// target as well as a clean up function that will be called once the
// test ends.
//
// Optionally, this function will update the golden files if the
// -update-golden flag was passed to the test.
func verifyProberMetrics(
	t *testing.T,
	name string,
	setup func(context.Context, *testing.T) (prober.Prober, sm.Check, func()),
	basicMetricsOnly bool,
) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	summaries := make(map[uint64]prometheus.Summary)
	histograms := make(map[uint64]prometheus.Histogram)
	logger := &testLogger{w: io.Discard}

	if os.Getenv("CI") == "true" {
		logger.w = os.Stdout
	}

	prober, check, stop := setup(ctx, t)
	defer stop()

	success, mfs, err := getProbeMetrics(
		ctx,
		prober,
		check.Target,
		timeout,
		nil,
		summaries,
		histograms,
		logger,
		basicMetricsOnly,
	)

	require.NoError(t, err, "probe failed")
	require.Truef(t, success, "probe failed: %s", prober.Name())

	fn := filepath.Join("testdata", name+".txt")

	if *updateGolden {
		var buf bytes.Buffer

		enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))

		for _, m := range mfs {
			if err := enc.Encode(m); err != nil {
				t.Fatalf("encoding golden file: %s", err.Error())
			}
		}

		fh, err := os.Create(fn)
		if err != nil {
			t.Fatalf("cannot create file %s: %s", fn, err.Error())
		}
		defer fh.Close()

		if _, err := buf.WriteTo(fh); err != nil {
			t.Fatalf("cannot write to file %s: %s", fn, err.Error())
		}

		return
	}

	actualMetrics := map[string]struct{}{}

	for _, m := range mfs {
		addMetricToIndex(m, actualMetrics)
	}

	expectedMetrics, err := readGoldenFile(fn)
	require.NoError(t, err, "reading golden file")

	require.Equal(t, expectedMetrics, actualMetrics, "maps must be equal")
}

func addMetricToIndex(mf *dto.MetricFamily, index map[string]struct{}) {
	for _, metric := range mf.GetMetric() {
		labels := make([]string, 0, len(metric.GetLabel()))
		for _, label := range metric.GetLabel() {
			labels = append(labels, fmt.Sprintf(`"%s"="%s"`, label.GetName(), label.GetValue()))
		}
		sort.Strings(labels)
		index[mf.GetName()+"{"+strings.Join(labels, ",")+"}"] = struct{}{}
	}
}

func readGoldenFile(fn string) (map[string]struct{}, error) {
	fh, err := os.Open(fn)
	if err != nil {
		return nil, fmt.Errorf("cannot open file %s: %w", fn, err)
	}
	defer fh.Close()

	dec := expfmt.NewDecoder(fh, expfmt.NewFormat(expfmt.TypeTextPlain))

	metrics := map[string]struct{}{}

	for {
		var mf dto.MetricFamily

		switch err := dec.Decode(&mf); {
		case errors.Is(err, io.EOF):
			return metrics, nil

		case err != nil:
			return nil, fmt.Errorf("cannot decode golden file %s: %w", fn, err)
		}

		addMetricToIndex(&mf, metrics)
	}
}

func setupPingProbe(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
	check := sm.Check{
		Target:  "127.0.0.1",
		Timeout: 2000,
		Settings: sm.CheckSettings{
			Ping: &sm.PingSettings{
				IpVersion: sm.IpVersion_V4,
			},
		},
	}

	prober, err := icmp.NewProber(check)
	if err != nil {
		t.Fatalf("cannot create ICMP prober: %s", err)
	}

	return prober, check, func() {}
}

func setupHTTPProbe(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
	httpSrv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	httpSrv.Start()

	check := sm.Check{
		Target:  httpSrv.URL,
		Timeout: 2000,
		Settings: sm.CheckSettings{
			Http: &sm.HttpSettings{
				IpVersion: sm.IpVersion_V4,
			},
		},
	}

	prober, err := httpProber.NewProber(
		ctx,
		check,
		zerolog.New(io.Discard),
		http.Header{},
	)
	if err != nil {
		t.Fatalf("cannot create HTTP prober: %s", err)
	}

	return prober, check, httpSrv.Close
}

func setupHTTPSSLProbe(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
	httpSrv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	httpSrv.StartTLS()

	check := sm.Check{
		Target:  httpSrv.URL,
		Timeout: 2000,
		Settings: sm.CheckSettings{
			Http: &sm.HttpSettings{
				IpVersion: sm.IpVersion_V4,
				TlsConfig: &sm.TLSConfig{
					InsecureSkipVerify: true,
				},
			},
		},
	}

	prober, err := httpProber.NewProber(
		ctx,
		check,
		zerolog.New(io.Discard),
		http.Header{},
	)
	if err != nil {
		t.Fatalf("cannot create HTTP prober: %s", err)
	}

	return prober, check, httpSrv.Close
}

func setupDNSProbe(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
	srv, clean := setupDNSServer(t)
	check := sm.Check{
		Target:  "example.org",
		Timeout: 2000,
		Settings: sm.CheckSettings{
			// target is "example.com"
			Dns: &sm.DnsSettings{
				Server:    srv,
				IpVersion: sm.IpVersion_V4,
				Protocol:  sm.DnsProtocol_UDP,
			},
		},
	}
	prober, err := dnsProber.NewProber(check)
	if err != nil {
		clean()
		t.Fatalf("cannot create DNS prober: %s", err)
	}
	return prober, check, clean
}

func setupTCPProbe(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
	srv, clean := setupTCPServer(t)
	check := sm.Check{
		Target:  srv,
		Timeout: 2000,
		Settings: sm.CheckSettings{
			Tcp: &sm.TcpSettings{
				IpVersion: sm.IpVersion_V4,
			},
		},
	}
	prober, err := tcp.NewProber(
		ctx,
		check,
		zerolog.New(io.Discard))
	if err != nil {
		clean()
		t.Fatalf("cannot create TCP prober: %s", err)
	}
	return prober, check, clean
}

func setupTCPSSLProbe(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
	srv, clean := setupTCPServerWithSSL(t)
	check := sm.Check{
		Target:  srv,
		Timeout: 2000,
		Settings: sm.CheckSettings{
			Tcp: &sm.TcpSettings{
				IpVersion: sm.IpVersion_V4,
				Tls:       true,
				TlsConfig: &sm.TLSConfig{
					InsecureSkipVerify: true,
				},
			},
		},
	}
	prober, err := tcp.NewProber(
		ctx,
		check,
		zerolog.New(io.Discard))
	if err != nil {
		clean()
		t.Fatalf("cannot create TCP prober: %s", err)
	}
	return prober, check, clean
}

func setupTracerouteProbe(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
	checkCap := func(set *cap.Set, v cap.Value) {
		if permitted, err := set.GetFlag(cap.Permitted, v); err != nil {
			t.Fatalf("cannot get %s flag: %s", v, err)
		} else if !permitted {
			t.Skipf("traceroute cannot run, process doesn't have %s capability", v)
		}
	}
	c := cap.GetProc()
	checkCap(c, cap.NET_ADMIN)
	checkCap(c, cap.NET_RAW)

	check := sm.Check{
		Target: "127.0.0.1",
		Settings: sm.CheckSettings{
			Traceroute: &sm.TracerouteSettings{},
		},
	}

	p, err := traceroute.NewProber(check, zerolog.New(io.Discard))
	if err != nil {
		t.Fatalf("cannot create traceroute prober %s", err)
	}

	return p, check, func() {}
}

func setupScriptedProbe(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
	httpSrv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	httpSrv.Start()

	check := sm.Check{
		Target:  httpSrv.URL,
		Timeout: 2000,
		Settings: sm.CheckSettings{
			Scripted: &sm.ScriptedSettings{
				Script: []byte(`export default function() {}`),
			},
		},
	}

	var runner k6runner.Runner

	if k6Path := os.Getenv("K6_PATH"); k6Path != "" {
		runner = k6runner.New(k6runner.RunnerOpts{Uri: k6Path})
	} else {
		runner = &testRunner{
			metrics: testhelper.MustReadFile(t, "testdata/k6.dat"),
			logs:    nil,
		}
	}

	prober, err := scripted.NewProber(
		ctx,
		check,
		zerolog.New(zerolog.NewTestWriter(t)),
		runner,
	)
	if err != nil {
		t.Fatalf("cannot create scripted prober: %s", err)
	}

	return prober, check, httpSrv.Close
}

func setupMultiHTTPProbe(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
	httpSrv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	httpSrv.Start()

	check := sm.Check{
		Target:  httpSrv.URL,
		Timeout: 2000,
		Settings: sm.CheckSettings{
			Multihttp: &sm.MultiHttpSettings{
				Entries: []*sm.MultiHttpEntry{
					{
						Request: &sm.MultiHttpEntryRequest{
							Method: sm.HttpMethod_GET,
							Url:    httpSrv.URL,
						},
					},
				},
			},
		},
	}

	var runner k6runner.Runner

	if k6Path := os.Getenv("K6_PATH"); k6Path != "" {
		runner = k6runner.New(k6runner.RunnerOpts{Uri: k6Path})
	} else {
		runner = &testRunner{
			metrics: testhelper.MustReadFile(t, "testdata/multihttp.dat"),
			logs:    nil,
		}
	}

	prober, err := multihttp.NewProber(
		ctx,
		check,
		zerolog.New(zerolog.NewTestWriter(t)),
		runner,
		http.Header{},
	)
	if err != nil {
		t.Fatalf("cannot create MultiHTTP prober: %s", err)
	}

	return prober, check, httpSrv.Close
}

func setupBrowserProbe(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
	httpSrv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	httpSrv.Start()

	check := sm.Check{
		Target:  httpSrv.URL,
		Timeout: 2000,
		Settings: sm.CheckSettings{
			Browser: &sm.BrowserSettings{
				Script: []byte(`export default function() {}`),
			},
		},
	}

	var runner k6runner.Runner

	if k6Path := os.Getenv("K6_PATH"); k6Path != "" {
		runner = k6runner.New(k6runner.RunnerOpts{Uri: k6Path})
	} else {
		runner = &testRunner{
			metrics: testhelper.MustReadFile(t, "testdata/browser.dat"),
			logs:    nil,
		}
	}

	prober, err := browser.NewProber(
		ctx,
		check,
		zerolog.New(zerolog.NewTestWriter(t)),
		runner,
	)
	if err != nil {
		t.Fatalf("cannot create scripted prober: %s", err)
	}

	return prober, check, httpSrv.Close
}

func setupGRPCProbe(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
	srv, clean := setupGRPCServer(t)
	check := sm.Check{
		Target:  srv,
		Timeout: 2000,
		Settings: sm.CheckSettings{
			Grpc: &sm.GrpcSettings{
				IpVersion: sm.IpVersion_V4,
			},
		},
	}
	prober, err := grpcProber.NewProber(
		ctx,
		check,
		zerolog.New(io.Discard))
	if err != nil {
		clean()
		t.Fatalf("cannot create gRPC prober: %s", err)
	}
	return prober, check, clean
}

func setupGRPCSSLProbe(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
	srv, clean := setupGRPCServerWithSSL(t)
	check := sm.Check{
		Target:  srv,
		Timeout: 2000,
		Settings: sm.CheckSettings{
			Grpc: &sm.GrpcSettings{
				IpVersion: sm.IpVersion_V4,
				Tls:       true,
				TlsConfig: &sm.TLSConfig{
					InsecureSkipVerify: true,
				},
			},
		},
	}
	prober, err := grpcProber.NewProber(
		ctx,
		check,
		zerolog.New(io.Discard))
	if err != nil {
		clean()
		t.Fatalf("cannot create gRPC prober: %s", err)
	}
	return prober, check, clean
}

func setupDNSServer(t *testing.T) (string, func()) {
	dnsSrv, dnsAddr := startDNSServer(":0", "udp", recursiveDNSHandler)

	errCh := make(chan error)

	go func() {
		dnsSrv.NotifyStartedFunc = func() {
			close(errCh)
		}
		err := dnsSrv.ActivateAndServe()
		if err != nil {
			errCh <- err
			close(errCh)
		}
	}()

	if err := <-errCh; err != nil {
		t.Fatalf("error activating DNS server: %s", err.Error())
	}

	return dnsAddr.String(), func() {
		err := dnsSrv.Shutdown()
		if err != nil {
			// this should never happen, but log it if it does
			t.Fatalf("error shutting down DNS server: %s", err.Error())
		}
	}
}

// startDNSServer starts a DNS server with a given handler function on a random port.
// Returns the Server object itself as well as the net.Addr corresponding to the server port.
func startDNSServer(addr, protocol string, handler func(dns.ResponseWriter, *dns.Msg)) (*dns.Server, net.Addr) {
	h := dns.NewServeMux()
	h.HandleFunc(".", handler)

	server := &dns.Server{Net: protocol, Handler: h}
	switch protocol {
	case "udp", "udp4", "udp6":
		a, err := net.ResolveUDPAddr(server.Net, server.Addr)
		if err != nil {
			panic(err)
		}
		l, err := net.ListenUDP(server.Net, a)
		if err != nil {
			panic(err)
		}
		server.Addr = l.LocalAddr().String()
		server.PacketConn = l
	case "tcp", "tcp4", "tcp6":
		a, err := net.ResolveTCPAddr(server.Net, server.Addr)
		if err != nil {
			panic(err)
		}
		l, err := net.ListenTCP(server.Net, a)
		if err != nil {
			panic(err)
		}
		server.Addr = l.Addr().String()
		server.Listener = l
	default:
		panic("unknown protocol")
	}

	if protocol == "tcp" {
		return server, server.Listener.Addr()
	}
	return server, server.PacketConn.LocalAddr()
}

func recursiveDNSHandler(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	answers := []string{
		"example.com. 3600 IN A 127.0.0.1",
		"example.com. 3600 IN A 127.0.0.2",
	}
	for _, rr := range answers {
		a, err := dns.NewRR(rr)
		if err != nil {
			panic(err)
		}
		m.Answer = append(m.Answer, a)
	}
	if err := w.WriteMsg(m); err != nil {
		panic(err)
	}
}

func setupTCPServer(t *testing.T) (string, func()) {
	ln, err := net.Listen("tcp4", ":0")
	if err != nil {
		t.Fatalf("Error listening on socket: %s", err)
	}

	done := make(chan (struct{}))

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			panic(fmt.Sprintf("Error accepting on socket: %s", err))
		}
		defer func() {
			conn.Close()
			close(done)
		}()

		buf := make([]byte, 0, 1024)
		_, _ = conn.Read(buf)
	}()

	return ln.Addr().String(), func() {
		<-done
		ln.Close()
	}
}

func setupTCPServerWithSSL(t *testing.T) (string, func()) {
	ln, err := net.Listen("tcp4", ":0")
	if err != nil {
		t.Fatalf("Error listening on socket: %s", err)
	}

	cert, err := tls.X509KeyPair(localhostCert, localhostKey)
	if err != nil {
		t.Fatalf("creating X509 key pair: %s", err.Error())
	}

	tlsCfg := tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"foo"},
	}

	ln = tls.NewListener(ln, &tlsCfg)

	done := make(chan (struct{}))

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			panic(fmt.Sprintf("Error accepting on socket: %s", err))
		}
		defer func() {
			conn.Close()
			close(done)
		}()

		buf := make([]byte, 0, 1024)
		_, _ = conn.Read(buf)
	}()

	return ln.Addr().String(), func() {
		<-done
		ln.Close()
	}
}

type gRPCSrv struct {
	grpchealth.HealthServer
}

func (s *gRPCSrv) Check(
	context.Context, *grpchealth.HealthCheckRequest,
) (*grpchealth.HealthCheckResponse, error) {
	return &grpchealth.HealthCheckResponse{
		Status: grpchealth.HealthCheckResponse_SERVING,
	}, nil
}

func setupGRPCServer(t *testing.T) (string, func()) {
	lis, err := net.Listen("tcp4", ":0")
	if err != nil {
		t.Fatalf("Error listening on socket: %v", err)
	}

	srv := grpc.NewServer()
	grpchealth.RegisterHealthServer(srv, &gRPCSrv{})

	go func() {
		_ = srv.Serve(lis)
	}()

	return lis.Addr().String(), func() {
		srv.Stop()
		lis.Close()
	}
}

func setupGRPCServerWithSSL(t *testing.T) (string, func()) {
	lis, err := net.Listen("tcp4", ":0")
	if err != nil {
		t.Fatalf("Error listening on socket: %v", err)
	}

	cert, err := tls.X509KeyPair(localhostCert, localhostKey)
	if err != nil {
		t.Fatalf("Error creating X509 key pair: %v", err)
	}

	tlsCfg := tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"foo"},
	}

	srv := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(&tlsCfg)),
	)
	grpchealth.RegisterHealthServer(srv, &gRPCSrv{})

	go func() {
		_ = srv.Serve(lis)
	}()

	return lis.Addr().String(), func() {
		srv.Stop()
		lis.Close()
	}
}

// TestValidateLabels validates that no probe generates more metric or log
// labels than the exported constants that specify these maximums.
// This test is required to be aware if any modification extends these
// maximums. These maximums are useful to calculate how many check labels can
// be set without exceeding the Mimir and Loki limits.
func TestValidateLabels(t *testing.T) {
	testCases := map[string]struct {
		setup func(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func())
	}{
		"ping": {
			setup: setupPingProbe,
		},
		"http": {
			setup: setupHTTPProbe,
		},
		"http_ssl": {
			setup: setupHTTPSSLProbe,
		},
		"dns": {
			setup: setupDNSProbe,
		},
		"tcp": {
			setup: setupTCPProbe,
		},
		"tcp_ssl": {
			setup: setupTCPSSLProbe,
		},
		"traceroute": {
			setup: setupTracerouteProbe,
		},
		"scripted": {
			setup: setupScriptedProbe,
		},
		"multihttp": {
			setup: setupMultiHTTPProbe,
		},
		"grpc": {
			setup: setupGRPCProbe,
		},
		"grpc_ssl": {
			setup: setupGRPCSSLProbe,
		},
	}

	type maxMetricLabels struct {
		AllMetrics      int
		CheckInfoMetric int
	}

	isCheckInfoMetric := func(labels []prompb.Label) bool {
		for _, l := range labels {
			if l.Name == "__name__" && l.Value == CheckInfoMetricName {
				return true
			}
		}
		return false
	}

	// maxProbeMetricLabels returns the maximum number of labels for any
	// timeseries from probeData, and also the maximum number of labels for the
	// sm_check_info metric (that should be fairly static across all checks,
	// only depends on alert sensitivity, but review all generated tss anyway).
	maxProbeMetricLabels := func(t *testing.T, tss TimeSeries) maxMetricLabels {
		max := 0
		maxCheckInfoMetric := 0
		for _, ts := range tss {
			labels := ts.GetLabels()
			nLabels := len(labels) - 1 // Discount special __name__ label
			if nLabels > max {
				max = nLabels
			}
			// Check for CheckInfoMetric
			if isCheckInfoMetric(labels) {
				if nLabels > maxCheckInfoMetric {
					maxCheckInfoMetric = nLabels
				}
			}
		}
		return maxMetricLabels{
			AllMetrics:      max,
			CheckInfoMetric: maxCheckInfoMetric,
		}
	}

	// maxProbeLogLabels returns the maximum number of labels for any
	// log stream from probeData
	maxProbeLogLabels := func(t *testing.T, ss Streams) int {
		max := 0
		for _, s := range ss {
			labels, err := parser.ParseMetric(s.Labels)
			require.NoError(t, err)

			nLabels := len(labels)
			if nLabels > max {
				max = nLabels
			}
		}
		return max
	}

	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var (
		totalMaxMetricLabels    int
		totalMaxCheckInfoLabels int
		totalMaxLogLabels       int
	)

	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			prober, check, stop := tc.setup(ctx, t)
			defer stop()

			check.AlertSensitivity = "low" // Force sm_check_info metric to include alert sensitivity label

			s := Scraper{
				checkName: "check name",
				target:    check.Target,
				logger:    zerolog.Nop(),
				prober:    prober,
				labelsLimiter: testLabelsLimiter{
					maxMetricLabels: 100,
					maxLogLabels:    100,
				},
				summaries:  make(map[uint64]prometheus.Summary),
				histograms: make(map[uint64]prometheus.Histogram),
				check: model.Check{
					Check: check,
				},
				probe: sm.Probe{
					Id:        100,
					TenantId:  200,
					Name:      "probe name",
					Latitude:  -1,
					Longitude: -2,
					Region:    "region",
				},
			}

			data, duration, err := s.collectData(context.Background(), time.Unix(int64(3141000)/1000, 0))
			require.NoError(t, err)
			require.NotNil(t, data)
			require.NotZero(t, duration)

			metricLabels := maxProbeMetricLabels(t, data.Metrics())
			logLabels := maxProbeLogLabels(t, data.Streams())

			if metricLabels.AllMetrics > totalMaxMetricLabels {
				totalMaxMetricLabels = metricLabels.AllMetrics
			}
			if metricLabels.CheckInfoMetric > totalMaxCheckInfoLabels {
				totalMaxCheckInfoLabels = metricLabels.CheckInfoMetric
			}
			if logLabels > totalMaxLogLabels {
				totalMaxLogLabels = logLabels
			}
		})
	}

	require.Equal(t, sm.MaxAgentMetricLabels(), totalMaxMetricLabels)
	require.Equal(t, sm.MaxAgentCheckInfoLabels(), totalMaxCheckInfoLabels)
	require.Equal(t, sm.MaxAgentLogLabels(), totalMaxLogLabels)
}

func TestMakeTimeseries(t *testing.T) {
	testTime := time.Now()
	testValue := 42.0
	testcases := map[string]struct {
		t      time.Time
		value  float64
		labels []prompb.Label
	}{
		"no labels": {
			t:      testTime,
			value:  testValue,
			labels: []prompb.Label{},
		},
		"one label": {
			t:     testTime,
			value: testValue,
			labels: []prompb.Label{
				{Name: "name", Value: "value"},
			},
		},
		"many labels": {
			t:     testTime,
			value: testValue,
			labels: []prompb.Label{
				{Name: "name 1", Value: "value 1"},
				{Name: "name 2", Value: "value 2"},
				{Name: "name 3", Value: "value 3"},
				{Name: "name 4", Value: "value 4"},
			},
		},
	}

	for name, tc := range testcases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			actual := makeTimeseries(tc.t, tc.value, tc.labels...)
			require.Equal(t, len(tc.labels), len(actual.Labels))
			require.Equal(t, tc.labels, actual.Labels)
			require.Equal(t, 1, len(actual.Samples))
			require.Equal(t, tc.t.UnixNano()/1e6, actual.Samples[0].Timestamp)
			require.Equal(t, tc.value, actual.Samples[0].Value)
		})
	}
}

func TestAppendDtoToTimeseries(t *testing.T) {
	makeUint64Ptr := func(n uint64) *uint64 {
		return &n
	}

	makeFloat64Ptr := func(n float64) *float64 {
		return &n
	}

	testTime := time.Now()
	testTimestamp := testTime.UnixNano() / 1e6
	testValue := 42.0
	testcases := map[string]struct {
		t            time.Time
		mName        string
		sharedLabels []prompb.Label
		mType        dto.MetricType
		metric       *dto.Metric
		expected     []prompb.TimeSeries
	}{
		"counter": {
			t:     testTime,
			mName: "test",
			sharedLabels: []prompb.Label{
				{Name: "label 1", Value: "value 1"},
			},
			mType: dto.MetricType_COUNTER,
			metric: &dto.Metric{
				Counter: &dto.Counter{
					Value: &testValue,
				},
			},
			expected: []prompb.TimeSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test"},
						{Name: "label 1", Value: "value 1"},
					},
					Samples: []prompb.Sample{
						{Timestamp: testTimestamp, Value: testValue},
					},
				},
			},
		},
		"gauge": {
			t:     testTime,
			mName: "test",
			sharedLabels: []prompb.Label{
				{Name: "label 1", Value: "value 1"},
			},
			mType: dto.MetricType_GAUGE,
			metric: &dto.Metric{
				Gauge: &dto.Gauge{
					Value: &testValue,
				},
			},
			expected: []prompb.TimeSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test"},
						{Name: "label 1", Value: "value 1"},
					},
					Samples: []prompb.Sample{
						{Timestamp: testTimestamp, Value: testValue},
					},
				},
			},
		},
		"untyped": {
			t:     testTime,
			mName: "test",
			sharedLabels: []prompb.Label{
				{Name: "label 1", Value: "value 1"},
			},
			mType: dto.MetricType_UNTYPED,
			metric: &dto.Metric{
				Untyped: &dto.Untyped{
					Value: &testValue,
				},
			},
			expected: []prompb.TimeSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test"},
						{Name: "label 1", Value: "value 1"},
					},
					Samples: []prompb.Sample{
						{Timestamp: testTimestamp, Value: testValue},
					},
				},
			},
		},
		"summary": {
			t:     testTime,
			mName: "test",
			sharedLabels: []prompb.Label{
				{Name: "label 1", Value: "value 1"},
			},
			mType: dto.MetricType_SUMMARY,
			metric: &dto.Metric{
				Summary: &dto.Summary{
					SampleCount: makeUint64Ptr(7),
					SampleSum:   makeFloat64Ptr(0.25),
				},
			},
			expected: []prompb.TimeSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_sum"},
						{Name: "label 1", Value: "value 1"},
					},
					Samples: []prompb.Sample{
						{Timestamp: testTimestamp, Value: 0.25},
					},
				},
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_count"},
						{Name: "label 1", Value: "value 1"},
					},
					Samples: []prompb.Sample{
						{Timestamp: testTimestamp, Value: 7},
					},
				},
			},
		},
		"histogram": {
			t:     testTime,
			mName: "test",
			sharedLabels: []prompb.Label{
				{Name: "label 1", Value: "value 1"},
			},
			mType: dto.MetricType_HISTOGRAM,
			metric: &dto.Metric{
				Histogram: &dto.Histogram{
					SampleCount: makeUint64Ptr(17),
					SampleSum:   makeFloat64Ptr(120),
					Bucket: []*dto.Bucket{
						{
							CumulativeCount: makeUint64Ptr(1),
							UpperBound:      makeFloat64Ptr(0.1),
						},
						{
							CumulativeCount: makeUint64Ptr(5),
							UpperBound:      makeFloat64Ptr(1.0),
						},
						{
							CumulativeCount: makeUint64Ptr(11),
							UpperBound:      makeFloat64Ptr(10.0),
						},
					},
				},
			},
			expected: []prompb.TimeSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_sum"},
						{Name: "label 1", Value: "value 1"},
					},
					Samples: []prompb.Sample{
						{Timestamp: testTimestamp, Value: 120},
					},
				},
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_count"},
						{Name: "label 1", Value: "value 1"},
					},
					Samples: []prompb.Sample{
						{Timestamp: testTimestamp, Value: 17},
					},
				},
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_bucket"},
						{Name: "label 1", Value: "value 1"},
						{Name: "le", Value: "0.1"},
					},
					Samples: []prompb.Sample{
						{Timestamp: testTimestamp, Value: 1},
					},
				},
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_bucket"},
						{Name: "label 1", Value: "value 1"},
						{Name: "le", Value: "1"},
					},
					Samples: []prompb.Sample{
						{Timestamp: testTimestamp, Value: 5},
					},
				},
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_bucket"},
						{Name: "label 1", Value: "value 1"},
						{Name: "le", Value: "10"},
					},
					Samples: []prompb.Sample{
						{Timestamp: testTimestamp, Value: 11},
					},
				},
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_bucket"},
						{Name: "label 1", Value: "value 1"},
						{Name: "le", Value: "+Inf"},
					},
					Samples: []prompb.Sample{
						{Timestamp: testTimestamp, Value: 17},
					},
				},
			},
		},
	}

	for name, tc := range testcases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			actual := appendDtoToTimeseries(
				nil,
				tc.t,
				tc.mName,
				tc.sharedLabels,
				tc.mType,
				tc.metric,
			)

			require.Equal(t, tc.expected, actual)
		})
	}
}

type testProber struct{}

func (p testProber) Name() string {
	return "test prober"
}

func (p testProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_counter",
	})
	registry.MustRegister(counter)
	counter.Inc()

	return true
}

type testLabelsLimiter struct {
	maxMetricLabels int
	maxLogLabels    int
}

func (l testLabelsLimiter) MetricLabels(ctx context.Context, tenantID model.GlobalID) (int, error) {
	return l.maxMetricLabels, nil
}

func (l testLabelsLimiter) LogLabels(ctx context.Context, tenantID model.GlobalID) (int, error) {
	return l.maxLogLabels, nil
}

//nolint:gocyclo
func TestScraperCollectData(t *testing.T) {
	const (
		checkName     = "check name"
		checkTarget   = "target name"
		frequency     = 2000
		job           = "job name"
		modifiedTs    = 42
		probeLatitude = -1
		probeLongitde = -2
		probeName     = "probe name"
		region        = "REGION"
		sampleTsMs    = int64(3141000)
	)

	var (
		baseExpectedMetricLabels = map[string]string{
			"config_version": strconv.Itoa(modifiedTs * 1_000_000_000),
			"instance":       checkTarget,
			"job":            job,
			"probe":          probeName,
			// "source":         CheckInfoSource,
		}
		baseExpectedInfoLabels = map[string]string{
			"check_name": checkName,
			"frequency":  strconv.Itoa(frequency),
			"geohash":    geohash.Encode(probeLatitude, probeLongitde),
			"region":     region,
		}
		baseExpectedLogLabels = map[string]string{
			"check_name":           checkName,
			"instance":             checkTarget,
			"job":                  job,
			"probe":                probeName,
			ProbeSuccessMetricName: "1",
			"region":               region,
			"source":               CheckInfoSource,
		}
	)

	generateLabels := func(offset, count int, valuePrefix string) []sm.Label {
		var labels []sm.Label
		for i := 0; i < count; i++ {
			n := strconv.Itoa(offset + i)
			labels = append(labels, sm.Label{
				Name:  "l" + n,
				Value: valuePrefix + n,
			})
		}
		return labels
	}

	generateLabelSet := func(offset, count int, valuePrefix string) map[string]string {
		labels := make(map[string]string)
		for i := 0; i < count; i++ {
			n := strconv.Itoa(offset + i)
			labels["label_l"+n] = valuePrefix + n
		}
		return labels
	}

	type testcase struct {
		maxMetricLabels      int
		maxLogLabels         int
		checkLabels          []sm.Label
		probeLabels          []sm.Label
		expectedMetricLabels map[string]string
		expectedInfoLabels   map[string]string
		expectedLogLabels    map[string]string
		expectedLogEntries   map[string]string
	}

	const (
		defMaxMetricLabels = 20
		defMaxLogLabels    = 15
	)

	testcases := map[string]testcase{
		"trivial": {
			maxMetricLabels:      defMaxMetricLabels,
			maxLogLabels:         defMaxLogLabels,
			expectedMetricLabels: mergeMaps(baseExpectedMetricLabels),
			expectedInfoLabels:   mergeMaps(baseExpectedMetricLabels, baseExpectedInfoLabels),
			expectedLogLabels:    mergeMaps(baseExpectedLogLabels),
			expectedLogEntries:   mergeMaps(baseExpectedLogLabels),
		},
		"probe labels": {
			maxMetricLabels:      defMaxMetricLabels,
			maxLogLabels:         defMaxLogLabels,
			probeLabels:          generateLabels(1, 3, "p"),
			expectedMetricLabels: mergeMaps(baseExpectedMetricLabels),
			expectedInfoLabels:   mergeMaps(baseExpectedMetricLabels, baseExpectedInfoLabels, generateLabelSet(1, 3, "p")),
			expectedLogLabels:    mergeMaps(baseExpectedLogLabels, generateLabelSet(1, 3, "p")),
			expectedLogEntries:   mergeMaps(baseExpectedLogLabels, generateLabelSet(1, 3, "p")),
		},
		"check labels": {
			maxMetricLabels:      defMaxMetricLabels,
			maxLogLabels:         defMaxLogLabels,
			checkLabels:          generateLabels(1, 3, "c"),
			expectedMetricLabels: mergeMaps(baseExpectedMetricLabels),
			expectedInfoLabels:   mergeMaps(baseExpectedMetricLabels, baseExpectedInfoLabels, generateLabelSet(1, 3, "c")),
			expectedLogLabels:    mergeMaps(baseExpectedLogLabels, generateLabelSet(1, 3, "c")),
			expectedLogEntries:   mergeMaps(baseExpectedLogLabels, generateLabelSet(1, 3, "c")),
		},
		"check and probe labels": {
			maxMetricLabels:      defMaxMetricLabels,
			maxLogLabels:         defMaxLogLabels,
			checkLabels:          generateLabels(1, 2, "c"),
			probeLabels:          generateLabels(3, 1, "p"),
			expectedMetricLabels: mergeMaps(baseExpectedMetricLabels),
			expectedInfoLabels:   mergeMaps(baseExpectedMetricLabels, baseExpectedInfoLabels, generateLabelSet(3, 1, "p"), generateLabelSet(1, 2, "c")),
			expectedLogLabels:    mergeMaps(baseExpectedLogLabels, generateLabelSet(3, 1, "p"), generateLabelSet(1, 2, "c")),
			expectedLogEntries:   mergeMaps(baseExpectedLogLabels, generateLabelSet(3, 1, "p"), generateLabelSet(1, 2, "c")),
		},
		"check and probe labels overlapping": {
			maxMetricLabels:      defMaxMetricLabels,
			maxLogLabels:         defMaxLogLabels,
			checkLabels:          generateLabels(1, 2, "c"),
			probeLabels:          generateLabels(2, 2, "p"),
			expectedMetricLabels: mergeMaps(baseExpectedMetricLabels),
			expectedInfoLabels:   mergeMaps(baseExpectedMetricLabels, baseExpectedInfoLabels, generateLabelSet(3, 1, "p"), generateLabelSet(1, 2, "c")),
			expectedLogLabels:    mergeMaps(baseExpectedLogLabels, generateLabelSet(3, 1, "p"), generateLabelSet(1, 2, "c")),
			expectedLogEntries:   mergeMaps(baseExpectedLogLabels, generateLabelSet(3, 1, "p"), generateLabelSet(1, 2, "c")),
		},
		"max labels truncate": {
			maxMetricLabels:      defMaxMetricLabels,
			maxLogLabels:         defMaxLogLabels,
			checkLabels:          generateLabels(0, 10, "c"),
			probeLabels:          generateLabels(10, 3, "p"),
			expectedMetricLabels: mergeMaps(baseExpectedMetricLabels),
			expectedInfoLabels:   mergeMaps(baseExpectedMetricLabels, baseExpectedInfoLabels, generateLabelSet(0, 10, "c"), generateLabelSet(10, 3, "p")),
			expectedLogLabels:    mergeMaps(baseExpectedLogLabels, generateLabelSet(0, 5, "c"), generateLabelSet(10, 3, "p")),
			expectedLogEntries:   mergeMaps(baseExpectedLogLabels, generateLabelSet(0, 10, "c"), generateLabelSet(10, 3, "p")),
		},
		"max labels override": {
			maxMetricLabels:      21,
			maxLogLabels:         21,
			checkLabels:          generateLabels(0, 10, "c"),
			probeLabels:          generateLabels(10, 4, "p"),
			expectedMetricLabels: mergeMaps(baseExpectedMetricLabels),
			expectedInfoLabels:   mergeMaps(baseExpectedMetricLabels, baseExpectedInfoLabels, generateLabelSet(0, 10, "c"), generateLabelSet(10, 4, "p")),
			expectedLogLabels:    mergeMaps(baseExpectedLogLabels, generateLabelSet(0, 10, "c"), generateLabelSet(10, 4, "p")),
			expectedLogEntries:   mergeMaps(baseExpectedLogLabels, generateLabelSet(0, 10, "c"), generateLabelSet(10, 4, "p")),
		},
	}

	getMetricName := func(t *testing.T, ts prompb.TimeSeries) string {
		for _, l := range ts.GetLabels() {
			if l.GetName() != labels.MetricName {
				continue
			}

			return l.GetValue()
		}

		require.Fail(t, "metric name not found")

		return ""
	}

	validateMetrics := func(t *testing.T, ts prompb.TimeSeries, tc testcase) {
		require.NotNil(t, ts)

		metricName := getMetricName(t, ts)

		actualLabels := make(map[string]string)
		actualLabelsCount := 0
		actualInfoLabels := make(map[string]string)
		actualInfoLabelsCount := 0

		// Verify that all the expected metric labels are present

		for _, l := range ts.GetLabels() {
			switch {
			case l.GetName() == labels.MetricName:
				// ignore

			case l.GetName() == labels.BucketLabel:
				// ignore

			case metricName == CheckInfoMetricName:
				expectedValue, isExpected := tc.expectedInfoLabels[l.GetName()]
				require.Truef(t, isExpected, "metric=%s label=%s value=%s", metricName, l.GetName(), l.GetValue())
				require.Equal(t, expectedValue, l.GetValue())
				actualInfoLabels[l.GetName()] = l.GetValue()
				actualInfoLabelsCount++

			default:
				expectedValue, isExpected := tc.expectedMetricLabels[l.GetName()]
				require.Truef(t, isExpected, "unexpected label: metric=%s label=%s value=%s", metricName, l.GetName(), l.GetValue())
				require.Equal(t, expectedValue, l.GetValue())
				actualLabels[l.GetName()] = l.GetValue()
				actualLabelsCount++
			}
		}

		if metricName == CheckInfoMetricName {
			require.Equal(t, tc.expectedInfoLabels, actualInfoLabels)
			require.Equal(t, len(tc.expectedInfoLabels), actualInfoLabelsCount)
		} else {
			require.Equal(t, tc.expectedMetricLabels, actualLabels)
			require.Equal(t, len(tc.expectedMetricLabels), actualLabelsCount)
		}

		for _, sample := range ts.GetSamples() {
			// This encodes the assumption that there's a single timestamp included in the
			// resulting metrics.
			require.Equal(t, sampleTsMs, sample.Timestamp)
		}
	}

	validateStreams := func(t *testing.T, s Scraper, stream logproto.Stream, tc testcase) {
		labels, err := parser.ParseMetric(stream.Labels)
		require.NoError(t, err)

		// Verify that all the expected log labels are present as labels in the stream labels.
		found := 0
		for _, label := range labels {
			expected, ok := tc.expectedLogLabels[label.Name]
			require.Truef(t, ok, "key=%s value=%s labels=%s", label.Name, label.Value, stream.Labels)
			require.Equalf(t, expected, label.Value, "key=%s", label.Name)
			found++
		}
		require.Equal(t, len(tc.expectedLogLabels), found, stream.Labels)

		// Verify that all the expected log labels are present as part of the actual log entry.
		for _, entry := range stream.Entries {
			dec := logfmt.NewDecoder(strings.NewReader(entry.Line))
			for dec.ScanRecord() {
				labelsFound := 1 // probe_success is NOT included in the log entry
				for dec.ScanKeyval() {
					key := string(dec.Key())
					val := string(dec.Value())
					switch key {
					case "level", "msg", "timeout_seconds", "duration_seconds":
					case "target":
						require.Equal(t, s.target, val)
					case "type":
						require.Equal(t, s.prober.Name(), val)
					default:
						expected, found := tc.expectedLogEntries[key]
						require.Truef(t, found, "key=%s value=%s", key, val)
						require.Equalf(t, expected, val, "key=%s", key)
						labelsFound++
					}
				}
				require.Equal(t, len(tc.expectedLogEntries), labelsFound)
			}
			require.NoError(t, dec.Err())
		}
	}

	for name, tc := range testcases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			s := Scraper{
				checkName: checkName,
				target:    "test target",
				logger:    zerolog.Nop(),
				prober:    testProber{},
				labelsLimiter: testLabelsLimiter{
					maxMetricLabels: tc.maxMetricLabels,
					maxLogLabels:    tc.maxLogLabels,
				},
				summaries:  make(map[uint64]prometheus.Summary),
				histograms: make(map[uint64]prometheus.Histogram),
				check: model.Check{
					Check: sm.Check{
						Id:               1,
						TenantId:         2,
						Frequency:        frequency,
						Timeout:          frequency,
						Enabled:          true,
						Target:           checkTarget,
						Job:              job,
						BasicMetricsOnly: true,
						Created:          modifiedTs,
						Modified:         modifiedTs,
						Labels:           tc.checkLabels,
						// [Check.Type] panics if all settings are nil. To work around that, we add an empty, non nil
						// HTTP settings section.
						Settings: sm.CheckSettings{
							Http: &sm.HttpSettings{},
						},
					},
				},
				probe: sm.Probe{
					Id:        100,
					TenantId:  200,
					Name:      probeName,
					Latitude:  probeLatitude,
					Longitude: probeLongitde,
					Region:    region,
					Labels:    tc.probeLabels,
				},
			}

			data, duration, err := s.collectData(context.Background(), time.Unix(sampleTsMs/1000, 0))
			require.NoError(t, err)
			require.NotNil(t, data)
			require.NotZero(t, duration)

			for _, ts := range data.Metrics() {
				validateMetrics(t, ts, tc)
			}

			for _, stream := range data.Streams() {
				validateStreams(t, s, stream, tc)
			}
		})
	}
}

// these are generated using
// go run $(go env GOROOT)/src/crypto/tls/generate_cert.go --rsa-bits 1024 --host 127.0.0.1,::1,example.com --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h

var localhostCert = []byte(`-----BEGIN CERTIFICATE-----
MIICFDCCAX2gAwIBAgIRAIoIa8inD4pBali2dBpc6+wwDQYJKoZIhvcNAQELBQAw
EjEQMA4GA1UEChMHQWNtZSBDbzAgFw03MDAxMDEwMDAwMDBaGA8yMDg0MDEyOTE2
MDAwMFowEjEQMA4GA1UEChMHQWNtZSBDbzCBnzANBgkqhkiG9w0BAQEFAAOBjQAw
gYkCgYEA5ii8ieKKayAtIX7BbKWB+zRDnJsFg02+cHwRy7nmX1t9MdObwNAMFIET
zvKz4Yctgi3WoyMZfp6pYugHHye829DbUsm+GI6Ca3zSzADfz+zL/nIWiJje0NB1
DXPASi03nTNk06O5RGlWeLqGKfyEY3xjcUrE7rKlNtLu8oK4Jm0CAwEAAaNoMGYw
DgYDVR0PAQH/BAQDAgKkMBMGA1UdJQQMMAoGCCsGAQUFBwMBMA8GA1UdEwEB/wQF
MAMBAf8wLgYDVR0RBCcwJYILZXhhbXBsZS5jb22HBH8AAAGHEAAAAAAAAAAAAAAA
AAAAAAEwDQYJKoZIhvcNAQELBQADgYEAyW2Yc/vnlO9PYcXw9ZRYt51hTCKCooGv
RwR1OwL9Kdr/eY+wGpO+YsXxdSmRcmg467TA5d1YpssyVceKRla22NgDoZ2psFTe
LqKfcZUN+jvtQMx4LnsRZcz2i2U35Biq4h0i3SOgROIOEjQJJ6I8wMw9jD5kS86Q
IYZvskPli5s=
-----END CERTIFICATE-----`)

var localhostKey = []byte(testingKey(`-----BEGIN RSA TESTING KEY-----
MIICdgIBADANBgkqhkiG9w0BAQEFAASCAmAwggJcAgEAAoGBAOYovIniimsgLSF+
wWylgfs0Q5ybBYNNvnB8Ecu55l9bfTHTm8DQDBSBE87ys+GHLYIt1qMjGX6eqWLo
Bx8nvNvQ21LJvhiOgmt80swA38/sy/5yFoiY3tDQdQ1zwEotN50zZNOjuURpVni6
hin8hGN8Y3FKxO6ypTbS7vKCuCZtAgMBAAECgYBzp1y2XOP5WL3U6wD/O1vJg0XG
WA+5H0Pm+jFnEg81M6ABfbfyd5jaZNIzV7oURf0UQTxt1aFmAwxS6w1JForLZn3g
PA3UVkDEZTl7C7h6kIY4PVzcki32V2YZ73e1zSCfvxIvbJ7SS697ua1sefIP5Gci
HNSRzanUyOKCZ1Or8QJBAPB1A/66Baydh+2nXddaad9d5Ifjvklk60tozLYpx7Il
R6NgU49Pa67MndGGykmbs094TYY8GswU2WoJVPUEYY8CQQD1CVB8++yK2jP0LkH/
s+3dag2S2IId5FwSbjiM8YtOFx2wN2HleX7Yr6ujPnKT+zy75eV0chPIP++QNKmy
eIJDAkAJm/OD63Usl8MF2UljwMY4We03DP/euPy6L772jKbhVKIPQls0f+0CuESa
SfOti15YD6uxcJd1jmO93A+cFwe7AkEAgLFnqHzXewWnC7PPzfA+GW+9uUYk8HYj
NTrWUI/7zgOuAALWU6M/z6ZTyuTdYIMvHrBblpDjeuS5eU9vYOCR6QJAWT7VShIT
sc8HRJOxUQWOI5hVUfLxVSosFDW7G5/75WlyeYwZOqOD54uNzavr8gpoTlvllUuP
GcRWUmpDEz0t6w==
-----END RSA TESTING KEY-----`))

func testingKey(s string) string { return strings.ReplaceAll(s, "TESTING KEY", "PRIVATE KEY") }

// testLogger impleents the Logger interface to pass it to the prober.
type testLogger struct {
	w io.Writer
}

func (l *testLogger) Log(kv ...interface{}) error {
	var buf strings.Builder

	for i, v := range kv {
		if i >= 2 && i%2 == 0 {
			buf.WriteString(", ")
		}

		switch v := v.(type) {
		case string:
			buf.WriteString(v)
			if i%2 == 0 {
				buf.WriteRune(':')
			}

		case error:
			buf.WriteString(v.Error())

		case interface{ String() string }:
			buf.WriteString(v.String())

		default:
			buf.WriteString(fmt.Sprintf("%#v", v))
		}
	}

	fmt.Fprintf(l.w, "%s\n", buf.String())

	return nil
}

func mergeMaps(maps ...map[string]string) map[string]string {
	out := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func TestTruncateLabelValue(t *testing.T) {
	testcases := map[string]struct {
		length         int
		expectedLength int
	}{
		"zero": {
			length:         0,
			expectedLength: 0,
		},
		"one": {
			length:         1,
			expectedLength: 1,
		},
		"max/2": {
			length:         maxLabelValueLength / 2,
			expectedLength: maxLabelValueLength / 2,
		},
		"max-1": {
			length:         maxLabelValueLength - 1,
			expectedLength: maxLabelValueLength - 1,
		},
		"max": {
			length:         maxLabelValueLength,
			expectedLength: maxLabelValueLength,
		},
		"max+1": {
			length:         maxLabelValueLength + 1,
			expectedLength: maxLabelValueLength,
		},
		"2*max": {
			length:         2 * maxLabelValueLength,
			expectedLength: maxLabelValueLength,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			input := strings.Repeat("a", tc.length)
			expected := strings.Repeat("a", tc.expectedLength)
			actual := truncateLabelValue(input)
			require.Equal(t, len(expected), len(actual))
			if tc.expectedLength < tc.length {
				require.Equal(t, expected[:len(expected)-3], actual[:len(actual)-3])
				require.Equal(t, "...", actual[len(actual)-3:])
			}
		})
	}
}

type testRunner struct {
	metrics []byte
	logs    []byte
}

var _ k6runner.Runner = &testRunner{}

func (r *testRunner) Run(ctx context.Context, script k6runner.Script) (*k6runner.RunResponse, error) {
	return &k6runner.RunResponse{
		Metrics: r.metrics,
		Logs:    r.logs,
	}, nil
}

func (r *testRunner) WithLogger(logger *zerolog.Logger) k6runner.Runner {
	return r
}

type testCounter struct {
	count atomic.Int32
}

func (c *testCounter) Inc() {
	c.count.Add(1)
}

type testCounterVec struct {
	counters map[string]Incrementer
	t        *testing.T
}

func (c *testCounterVec) WithLabelValues(v ...string) Incrementer {
	require.Len(c.t, v, 1)

	if _, found := c.counters[v[0]]; !found {
		c.counters[v[0]] = &testCounter{}
	}

	return c.counters[v[0]]
}

type testProberB struct {
	wantedFailures int32
	execCount      int32
	failureCount   int32
}

func (p testProberB) Name() string {
	return "test prober"
}

func (p *testProberB) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	p.execCount++

	if p.failureCount < p.wantedFailures {
		p.failureCount++
		return false
	}

	return true
}

type testProbeFactory struct {
	builder func() prober.Prober
}

func (f testProbeFactory) New(ctx context.Context, logger zerolog.Logger, check model.Check) (prober.Prober, string, error) {
	return f.builder(), check.Target, nil
}

type testPublisher struct{}

func (testPublisher) Publish(pusher.Payload) {
}

type testTelemeter struct {
	execCount int32
	ee        []telemetry.Execution
}

func (t *testTelemeter) AddExecution(e telemetry.Execution) {
	t.execCount++
}

// TestScraperRun will set up a scraper in such a way that it runs 5 times, and fails 2 out of those 5.
//
// This checks that the probe gets run, and that the metrics are correctly collected.
func TestScraperRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	t.Cleanup(cancel)

	var check model.Check
	err := check.FromSM(sm.Check{
		Id:        1,
		TenantId:  1000,
		Frequency: 100,
		Timeout:   1000,
		Enabled:   true,
		Target:    "127.0.0.1",
		Job:       "test",
		Settings: sm.CheckSettings{
			Ping: &sm.PingSettings{},
		},
	})
	require.NoError(t, err)

	var counter testCounter
	errCounter := testCounterVec{counters: make(map[string]Incrementer), t: t}

	testProber := &testProberB{wantedFailures: 2}
	testTelemeter := &testTelemeter{}

	metrics := NewMetrics(&counter, &errCounter)

	s, err := NewWithOpts(ctx, check, ScraperOpts{
		Metrics:      metrics,
		ProbeFactory: testProbeFactory{builder: func() prober.Prober { return testProber }},
		Logger:       zerolog.New(zerolog.NewTestWriter(t)),
		Publisher:    &testPublisher{},
		LabelsLimiter: testLabelsLimiter{
			maxMetricLabels: 20,
			maxLogLabels:    15,
		},
		Telemeter: testTelemeter,
	})

	require.NoError(t, err)
	require.NotNil(t, s)

	s.Run(ctx)

	require.Equal(t, testProber.execCount, counter.count.Load())
	require.Len(t, errCounter.counters, 1)
	checkErrCounter, found := errCounter.counters["check"]
	require.True(t, found)
	require.Equal(t, testProber.failureCount, checkErrCounter.(*testCounter).count.Load())

	// Verify telemetry
	require.Equal(t, counter.count.Load(), testTelemeter.execCount)
	for _, e := range testTelemeter.ee {
		require.Equal(t, 1000, e.LocalTenantID)
		require.Equal(t, check.Class(), e.CheckClass)
		require.Greater(t, 0, e.Duration)
	}
}

func TestTickWithOffset(t *testing.T) {
	const (
		WORK    = 1
		IDLE    = 2
		CLEANUP = 3
	)

	testcases := map[string]struct {
		timeout  time.Duration
		period   time.Duration
		offset   time.Duration
		maxIdle  time.Duration
		minGap   time.Duration
		expected []int
	}{
		"An idle worker running between regular runs": {
			timeout: 1050 * time.Millisecond,
			period:  500 * time.Millisecond,
			offset:  1,
			maxIdle: 100 * time.Millisecond,
			minGap:  50 * time.Millisecond,
			expected: []int{
				WORK, // 0
				IDLE, // 100
				IDLE, // 200
				IDLE, // 300
				IDLE, // 400
				WORK, // 500
				IDLE, // 600
				IDLE, // 700
				IDLE, // 800
				IDLE, // 900
				WORK, // 1000
				CLEANUP,
			},
		},
		"An idle worker trying to run within gap duration of regular runs.": {
			timeout: 1050 * time.Millisecond,
			period:  500 * time.Millisecond,
			offset:  1,
			maxIdle: 100 * time.Millisecond,
			minGap:  150 * time.Millisecond,
			expected: []int{
				WORK, // 0
				IDLE, // 200, there's no idle at 100 because it's too close to the previous run
				IDLE, // 300
				WORK, // 500, there's no idle at 400 because it's too close to the next run
				IDLE, // 700, there's no idle at 600 because it's too close to the previous run
				IDLE, // 800
				WORK, // 1000, there's no idle at 900 because it's too close to the next run
				CLEANUP,
			},
		},
		"A zero offset and a scraper that has already been cancelled.": {
			timeout:  0, // this asks for immediate cancellation
			period:   500 * time.Millisecond,
			offset:   0,
			maxIdle:  100 * time.Millisecond,
			minGap:   150 * time.Millisecond,
			expected: nil,
		},
		"A zero offset and a short timeout.": {
			timeout: 25 * time.Millisecond,
			period:  50 * time.Millisecond,
			offset:  0,
			maxIdle: 100 * time.Millisecond,
			minGap:  150 * time.Millisecond,
			expected: []int{
				WORK, // 0
				CLEANUP,
			},
		},
		"A zero offset and a larger timeout.": {
			timeout: 200 * time.Millisecond,
			period:  75 * time.Millisecond,
			offset:  0,
			maxIdle: 100 * time.Millisecond,
			minGap:  150 * time.Millisecond,
			expected: []int{
				WORK, // 0
				WORK, // 75
				WORK, // 150
				CLEANUP,
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := testhelper.Context(context.Background(), t)
			t.Cleanup(cancel)

			// Append a particular marker to the results slice
			// depending on which function was called by
			// TestTickWithOffset. This allows us to test that the
			// correct function is being called in the correct
			// order.
			var results []int

			work := func(context.Context, time.Time) { results = append(results, WORK) }
			idle := func(context.Context, time.Time) { results = append(results, IDLE) }
			cleanup := func(context.Context, time.Time) { results = append(results, CLEANUP) }

			stop := make(chan struct{})

			if tc.timeout > 0 {
				go func() {
					time.Sleep(tc.timeout)
					close(stop)
				}()
			} else {
				// why not in the goroutine? Because we need to
				// make sure the channel is closed *before*
				// calling tickWithOffset
				close(stop)
			}

			tickWithOffset(ctx, stop, work, idle, cleanup, tc.period, tc.offset, tc.maxIdle, tc.minGap)

			require.Equal(t, tc.expected, results)
		})
	}
}
