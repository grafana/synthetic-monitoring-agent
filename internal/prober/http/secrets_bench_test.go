package http

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/http/testserver"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

type benchPEMs struct {
	cert, key, bundle []byte
}

var (
	pemsOnce sync.Once
	pems     benchPEMs
)

func genCert(b *testing.B, cn string) ([]byte, []byte) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		b.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		DNSNames:              []string{cn},
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		b.Fatal(err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		b.Fatal(err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

// getPEMs returns a real RSA-2048 cert and key, and a bundle of at least 8 KiB of certs.
func getPEMs(b *testing.B) benchPEMs {
	pemsOnce.Do(func() {
		pems.cert, pems.key = genCert(b, "client.example.com")

		for len(pems.bundle) < 8*1024 {
			c, _ := genCert(b, "ca.example.com")
			pems.bundle = append(pems.bundle, c...)
		}
	})

	return pems
}

type benchShape struct {
	name     string
	settings *sm.HttpSettings
}

func benchShapes(p benchPEMs) []benchShape {
	return []benchShape{
		{"no-auth", &sm.HttpSettings{Method: sm.HttpMethod_GET}},
		{"bearer+basic", &sm.HttpSettings{
			Method:      sm.HttpMethod_GET,
			BearerToken: "glc_" + strings.Repeat("x", 60),
			BasicAuth:   &sm.BasicAuth{Username: "user", Password: "correct-horse-battery-staple"},
		}},
		{"mtls", &sm.HttpSettings{
			Method:    sm.HttpMethod_GET,
			TlsConfig: &sm.TLSConfig{CACert: p.cert, ClientCert: p.cert, ClientKey: p.key},
		}},
		{"ca-bundle-8k", &sm.HttpSettings{
			Method:    sm.HttpMethod_GET,
			TlsConfig: &sm.TLSConfig{CACert: p.bundle},
		}},
		{"ref-bearer+basic", &sm.HttpSettings{
			Method:      sm.HttpMethod_GET,
			BearerToken: "${secrets.api-token}",
			BasicAuth:   &sm.BasicAuth{Username: "user", Password: "${secrets.basic-pass}"},
		}},
		{"ref-mtls", &sm.HttpSettings{
			Method: sm.HttpMethod_GET,
			TlsConfig: &sm.TLSConfig{
				CACert:     []byte("${secrets.ca-crt}"),
				ClientCert: []byte("${secrets.client-crt}"),
				ClientKey:  []byte("${secrets.client-key}"),
			},
		}},
		{"ref-in-ca-bundle-8k", &sm.HttpSettings{
			Method:    sm.HttpMethod_GET,
			TlsConfig: &sm.TLSConfig{CACert: append(append([]byte{}, p.bundle...), "${secrets.ca-crt}"...)},
		}},
	}
}

// benchSecrets stands in for the provider's cache on a hit.
func benchSecrets(p benchPEMs) *testhelper.MockSecretProvider {
	return testhelper.NewMockSecretProvider(map[string]string{
		"api-token":  "glc_" + strings.Repeat("y", 60),
		"basic-pass": "correct-horse-battery-staple",
		"ca-crt":     string(p.cert),
		"client-crt": string(p.cert),
		"client-key": string(p.key),
	})
}

func benchProber(b *testing.B, p benchPEMs, settings *sm.HttpSettings, target string) Prober {
	check := model.Check{Check: sm.Check{
		Id: 1, TenantId: 1, Frequency: 10000, Timeout: 1000, Enabled: true,
		Target: target, Job: "bench", Probes: []int64{1}, BasicMetricsOnly: true,
		Settings: sm.CheckSettings{Http: settings},
	}}

	zl := zerolog.New(io.Discard).Level(zerolog.InfoLevel)

	prober, err := NewProber(context.Background(), check, zl, http.Header{}, benchSecrets(p))
	if err != nil {
		b.Fatal(err)
	}

	return prober
}

// BenchmarkBuildProbeConfig measures the config build every probe run starts with. Each iteration
// gets its own context, as the scraper does; cancelling it removes the TLS temp files.
func BenchmarkBuildProbeConfig(b *testing.B) {
	p := getPEMs(b)

	for _, s := range benchShapes(p) {
		b.Run(s.name, func(b *testing.B) {
			prober := benchProber(b, p, s.settings, "http://127.0.0.1/")

			b.ReportAllocs()

			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())

				if _, err := prober.buildProbeConfig(ctx); err != nil {
					b.Fatal(err)
				}

				cancel()
			}
		})
	}
}

// BenchmarkProbe measures a whole probe run against a loopback server.
func BenchmarkProbe(b *testing.B) {
	p := getPEMs(b)

	srv := testserver.New(testserver.Config{})
	defer srv.Close()

	target := testserver.Settings{Method: "GET", Status: 200}.URL(srv.Listener.Addr().String())
	kl := log.NewNopLogger()

	for _, s := range benchShapes(p) {
		b.Run(s.name, func(b *testing.B) {
			prober := benchProber(b, p, s.settings, target)

			b.ReportAllocs()

			for b.Loop() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)

				if ok, _ := prober.Probe(ctx, target, prometheus.NewRegistry(), kl, "bench"); !ok {
					b.Fatal("probe failed")
				}

				cancel()
			}
		})
	}
}
