package dns

import (
	"bytes"
	"context"
	"net"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/dns/internal/bbe/config"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	name := Prober.Name(Prober{})
	require.Equal(t, name, "dns")
}

func TestNewProber(t *testing.T) {
	testcases := map[string]struct {
		input       model.Check
		expected    Prober
		ExpectError bool
	}{
		"default": {
			input: model.Check{Check: sm.Check{
				Target: "www.grafana.com",
				Settings: sm.CheckSettings{
					Dns: &sm.DnsSettings{},
				},
			}},
			expected: Prober{
				config: config.Module{
					Prober:  "dns",
					Timeout: 0,
					DNS: config.DNSProbe{
						IPProtocol:         "ip6",
						IPProtocolFallback: true,
						TransportProtocol:  "tcp",
						QueryName:          "www.grafana.com",
						QueryType:          "ANY",
						Recursion:          true,
					},
				},
			},
			ExpectError: false,
		},
		"no-settings": {
			input: model.Check{
				Check: sm.Check{
					Target: "www.grafana.com",
					Settings: sm.CheckSettings{
						Dns: nil,
					},
				},
			},
			expected:    Prober{},
			ExpectError: true,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := NewProber(testcase.input)
			require.Equal(t, &testcase.expected, &actual)
			if testcase.ExpectError {
				require.Error(t, err, "unsupported check")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSettingsToModule(t *testing.T) {
	testcases := map[string]struct {
		input    sm.DnsSettings
		expected config.Module
	}{
		"default": {
			input: sm.DnsSettings{},
			expected: config.Module{
				Prober:  "dns",
				Timeout: 0,
				DNS: config.DNSProbe{
					IPProtocol:         "ip6",
					IPProtocolFallback: true,
					TransportProtocol:  "tcp",
					QueryName:          "www.grafana.com",
					QueryType:          "ANY",
					Recursion:          true,
				},
			},
		},
		"partial-settings": {
			input: sm.DnsSettings{
				RecordType: 4,
				Protocol:   1,
			},
			expected: config.Module{
				Prober:  "dns",
				Timeout: 0,
				DNS: config.DNSProbe{
					IPProtocol:         "ip6",
					IPProtocolFallback: true,
					TransportProtocol:  "udp",
					QueryName:          "www.grafana.com",
					QueryType:          "MX",
					Recursion:          true,
				},
			},
		},
		"validations": {
			input: sm.DnsSettings{
				RecordType: 4,
				Protocol:   1,
				ValidateAnswer: &sm.DNSRRValidator{
					FailIfMatchesRegexp:    []string{"test"},
					FailIfNotMatchesRegexp: []string{"not test"},
				},
				ValidateAuthority: &sm.DNSRRValidator{
					FailIfMatchesRegexp:    []string{"test"},
					FailIfNotMatchesRegexp: []string{"not test"},
				},
				ValidateAdditional: &sm.DNSRRValidator{
					FailIfMatchesRegexp:    []string{"test"},
					FailIfNotMatchesRegexp: []string{"not test"},
				},
			},
			expected: config.Module{
				Prober:  "dns",
				Timeout: 0,
				DNS: config.DNSProbe{
					IPProtocol:         "ip6",
					IPProtocolFallback: true,
					TransportProtocol:  "udp",
					QueryName:          "www.grafana.com",
					QueryType:          "MX",
					Recursion:          true,
					ValidateAnswer: config.DNSRRValidator{
						FailIfMatchesRegexp:    []string{"test"},
						FailIfNotMatchesRegexp: []string{"not test"},
					},
					ValidateAuthority: config.DNSRRValidator{
						FailIfMatchesRegexp:    []string{"test"},
						FailIfNotMatchesRegexp: []string{"not test"},
					},
					ValidateAdditional: config.DNSRRValidator{
						FailIfMatchesRegexp:    []string{"test"},
						FailIfNotMatchesRegexp: []string{"not test"},
					},
				},
			},
		},
	}

	for name, testcase := range testcases {
		target := "www.grafana.com"
		t.Run(name, func(t *testing.T) {
			actual := settingsToModule(&testcase.input, target)
			require.Equal(t, &testcase.expected, &actual)
		})
	}
}

func TestProberRetries(t *testing.T) {
	if !slices.Contains(strings.Split(os.Getenv("SM_TEST_RUN"), ","), "TestProberRetries") {
		t.Skip("Skipping long test TestProberRetries")
	}

	mux := dns.NewServeMux()
	var counter atomic.Int32
	mux.Handle(".", dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		answer := dns.Msg{
			MsgHdr: dns.MsgHdr{
				Id:       r.Id,
				Response: true,
				Rcode:    dns.RcodeSuccess,
			},
			Question: r.Question,
			Answer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   r.Question[0].Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					A: net.ParseIP("1.2.3.4"),
				},
			},
		}

		counter.Add(1)
		t.Logf("Received request %d: %v", counter.Load(), r)
		delay := 10 * time.Second
		if counter.Load()%3 == 0 {
			delay = 0
		}
		t.Log("Answer...")
		time.Sleep(delay)
		_ = w.WriteMsg(&answer)
	}))
	addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	l, err := net.ListenUDP("udp4", addr)
	require.NoError(t, err)
	t.Log(l.LocalAddr().String())
	server := &dns.Server{Addr: ":0", PacketConn: l, Net: "udp", Handler: mux}
	go func() {
		err := server.ActivateAndServe()
		if err != nil {
			panic(err)
		}
	}()

	p, err := NewExperimentalProber(model.Check{
		Check: sm.Check{
			Target:  "www.grafana.com",
			Timeout: 20000,
			Settings: sm.CheckSettings{
				Dns: &sm.DnsSettings{
					Server:     l.LocalAddr().String(),
					RecordType: sm.DnsRecordType_A,
					Protocol:   sm.DnsProtocol_UDP,
					IpVersion:  sm.IpVersion_V4,
				},
			},
		},
	})

	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), p.config.Timeout)
	t.Cleanup(cancel)

	defer func() {
		err := server.ShutdownContext(ctx)
		if err != nil {
			panic(err)
		}
	}()

	registry := prometheus.NewPedanticRegistry()

	var buf bytes.Buffer
	logger := log.NewLogfmtLogger(&buf)

	t0 := time.Now()
	success, duration := p.Probe(ctx, p.target, registry, logger)
	t.Log(success, time.Since(t0))
	require.True(t, success)
	require.Equal(t, 0, duration)

	mfs, err := registry.Gather()
	require.NoError(t, err)
	enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		require.NoError(t, enc.Encode(mf))
	}

	t.Log(buf.String())
}
