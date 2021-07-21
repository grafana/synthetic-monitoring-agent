package scraper

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	dnsProber "github.com/grafana/synthetic-monitoring-agent/internal/prober/dns"
	httpProber "github.com/grafana/synthetic-monitoring-agent/internal/prober/http"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/icmp"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/tcp"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/traceroute"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
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
			setup: func(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
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
			},
		},

		"http": {
			setup: func(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
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
				)
				if err != nil {
					t.Fatalf("cannot create HTTP prober: %s", err)
				}

				return prober, check, httpSrv.Close
			},
		},

		"http_ssl": {
			setup: func(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
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
				)
				if err != nil {
					t.Fatalf("cannot create HTTP prober: %s", err)
				}

				return prober, check, httpSrv.Close
			},
		},

		"dns": {
			setup: func(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
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
			},
		},

		"tcp": {
			setup: func(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
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
			},
		},

		"tcp_ssl": {
			setup: func(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
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
			},
		},

		"traceroute": {
			setup: func(ctx context.Context, t *testing.T) (prober.Prober, sm.Check, func()) {
				httpSrv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				httpSrv.Start()

				check := sm.Check{
					Target: httpSrv.URL,
					Settings: sm.CheckSettings{
						Traceroute: &sm.TracerouteSettings{},
					},
				}

				p, err := traceroute.NewProber(check)
				if err != nil {
					t.Fatalf("cannot create traceroute prober %s", err)
				}
				return p, check, func() {}
			},
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
	logger := &testLogger{w: ioutil.Discard}

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
	if err != nil {
		t.Fatalf("probe failed: %s", err.Error())
	} else if !success {
		t.Logf("probe failed: %s", prober.Name())
	}

	fn := filepath.Join("testdata", name+".txt")

	if *updateGolden {
		var buf bytes.Buffer

		enc := expfmt.NewEncoder(&buf, expfmt.FmtText)

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
	if err != nil {
		t.Fatal(err.Error())
	}

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

	dec := expfmt.NewDecoder(fh, expfmt.FmtText)

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

func setupDNSServer(t *testing.T) (string, func()) {
	dnsSrv, dnsAddr := startDNSServer(":0", "udp6", recursiveDNSHandler)

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

	server := &dns.Server{Addr: addr, Net: protocol, Handler: h}
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
		_, _ = conn.Read(buf[:])
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
		_, _ = conn.Read(buf[:])
	}()

	return ln.Addr().String(), func() {
		<-done
		ln.Close()
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
