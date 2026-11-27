package grpc

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"

	kitlog "github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	"github.com/prometheus/client_golang/prometheus"
	promcfg "github.com/prometheus/common/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestName(t *testing.T) {
	name := Prober.Name(Prober{})
	require.Equal(t, name, "grpc")
}

func TestNewProber(t *testing.T) {
	testcases := map[string]struct {
		input       model.Check
		expected    Prober
		ExpectError bool
	}{
		"default": {
			input: model.Check{
				Check: sm.Check{
					Target: "www.grafana.com",
					Settings: sm.CheckSettings{
						Grpc: &sm.GrpcSettings{},
					},
				},
			},
			expected: Prober{
				config: config.Module{
					Prober:  "grpc",
					Timeout: 0,
					GRPC: config.GRPCProbe{
						PreferredIPProtocol: "ip6",
						IPProtocolFallback:  true,
					},
				},
			},
		},
		"no-settings": {
			input: model.Check{
				Check: sm.Check{
					Target: "www.grafana.com",
					Settings: sm.CheckSettings{
						Grpc: nil,
					},
				},
			},
			ExpectError: true,
		},
	}

	ctx := context.Background()
	logger := zerolog.New(io.Discard)

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := NewProber(ctx, tc.input, logger, testhelper.NoopSecretStore{})

			require.Equal(t, &tc.expected, &actual)

			if tc.ExpectError {
				require.Error(t, err, "unsupported check")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSettingsToModule(t *testing.T) {
	testcases := map[string]struct {
		input    sm.GrpcSettings
		expected config.Module
	}{
		"default": {
			input: sm.GrpcSettings{},
			expected: config.Module{
				Prober:  "grpc",
				Timeout: 0,
				GRPC: config.GRPCProbe{
					PreferredIPProtocol: "ip6",
					IPProtocolFallback:  true,
				},
			},
		},
		"custom svc": {
			input: sm.GrpcSettings{
				Service: "customSvc",
			},
			expected: config.Module{
				Prober:  "grpc",
				Timeout: 0,
				GRPC: config.GRPCProbe{
					Service:             "customSvc",
					PreferredIPProtocol: "ip6",
					IPProtocolFallback:  true,
				},
			},
		},
		"ipv4": {
			input: sm.GrpcSettings{
				IpVersion: sm.IpVersion_V4,
			},
			expected: config.Module{
				Prober:  "grpc",
				Timeout: 0,
				GRPC: config.GRPCProbe{
					PreferredIPProtocol: "ip4",
					IPProtocolFallback:  false,
				},
			},
		},
		"tls": {
			input: sm.GrpcSettings{
				Tls: true,
				TlsConfig: &sm.TLSConfig{
					InsecureSkipVerify: true,
					ServerName:         "grafana.com",
				},
			},
			expected: config.Module{
				Prober:  "grpc",
				Timeout: 0,
				GRPC: config.GRPCProbe{
					PreferredIPProtocol: "ip6",
					IPProtocolFallback:  true,
					TLS:                 true,
					TLSConfig: promcfg.TLSConfig{
						InsecureSkipVerify: true,
						ServerName:         "grafana.com",
					},
				},
			},
		},
	}

	ctx := context.Background()
	logger := zerolog.New(io.Discard)

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := settingsToModule(ctx, &tc.input, logger)
			require.NoError(t, err)
			require.Equal(t, &tc.expected, &actual)
		})
	}
}

// TestProbeResolvesTLSSecrets runs against a server that requires a client certificate, so the
// run succeeds only if the CA cert, client cert and client key all resolve.
func TestProbeResolvesTLSSecrets(t *testing.T) {
	certPEM, keyPEM, serverTLS := testhelper.MutualTLS(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpclib.NewServer(grpclib.Creds(credentials.NewTLS(serverTLS)))
	healthpb.RegisterHealthServer(server, health.NewServer())

	go func() { _ = server.Serve(listener) }()

	t.Cleanup(server.Stop)

	secretStore := testhelper.NewMockSecretProvider(map[string]string{
		"ca":   string(certPEM),
		"cert": string(certPEM),
		"key":  string(keyPEM),
	})

	// A piece of the key body: zerolog escapes the newlines, so the whole PEM would never match.
	keyBody := string(bytes.Split(keyPEM, []byte("\n"))[1])

	testcases := map[string]struct {
		clientKey    string
		wantSuccess  bool
		wantCheckLog []string
	}{
		"every field from a secret": {
			clientKey:   "${secrets.key}",
			wantSuccess: true,
		},
		"missing secret": {
			clientKey:    "${secrets.missing}",
			wantCheckLog: []string{`field="client key"`, "secret=missing", secrets.ErrSecretNotFound.Error()},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := testhelper.Context(context.Background(), t)
			t.Cleanup(cancel)

			check := model.Check{Check: sm.Check{
				Target:  listener.Addr().String(),
				Timeout: 5000,
				Settings: sm.CheckSettings{Grpc: &sm.GrpcSettings{
					IpVersion: sm.IpVersion_V4,
					Tls:       true,
					TlsConfig: &sm.TLSConfig{
						CACert:     []byte("${secrets.ca}"),
						ClientCert: []byte("${secrets.cert}"),
						ClientKey:  []byte(tc.clientKey),
					},
				}},
			}}

			var (
				agentLog testhelper.LogBuffer
				checkLog bytes.Buffer
			)

			prober, err := NewProber(ctx, check, zerolog.New(&agentLog), secretStore)
			require.NoError(t, err)

			success, _ := prober.Probe(ctx, check.Target, prometheus.NewRegistry(), kitlog.NewLogfmtLogger(&checkLog), "")
			require.Equal(t, tc.wantSuccess, success, checkLog.String())

			for _, want := range tc.wantCheckLog {
				require.Contains(t, checkLog.String(), want)
			}

			require.NotContains(t, checkLog.String(), keyBody)
			require.NotContains(t, agentLog.String(), keyBody)
		})
	}
}
