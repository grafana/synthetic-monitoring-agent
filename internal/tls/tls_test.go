package tls

import (
	"os"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/interpolation"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/require"
)

func TestResolveSMtoProm(t *testing.T) {
	ctx, logger, tenantID := testhelper.CommonTestSetup()

	const (
		secretCACert = "-----BEGIN CERTIFICATE-----\nCA_CERT_CONTENT\n-----END CERTIFICATE-----"
		plainCACert  = "-----BEGIN CERTIFICATE-----\nPLAIN_CA_CERT\n-----END CERTIFICATE-----"
	)

	// Mock secret store that returns known values
	mockSecretStore := testhelper.NewMockSecretProvider(map[string]string{
		"ca-cert-key":     secretCACert,
		"client-cert-key": "-----BEGIN CERTIFICATE-----\nCLIENT_CERT_CONTENT\n-----END CERTIFICATE-----",
		"client-key-key":  "-----BEGIN PRIVATE KEY-----\nCLIENT_KEY_CONTENT\n-----END PRIVATE KEY-----",
	})

	testcases := map[string]struct {
		tlsConfig *sm.TLSConfig
		wantCA    string
	}{
		"TLS with secret interpolation": {
			tlsConfig: &sm.TLSConfig{
				InsecureSkipVerify: false,
				ServerName:         "example.com",
				CACert:             []byte("${secrets.ca-cert-key}"),
				ClientCert:         []byte("${secrets.client-cert-key}"),
				ClientKey:          []byte("${secrets.client-key-key}"),
			},
			wantCA: secretCACert,
		},
		"TLS with plain values": {
			tlsConfig: &sm.TLSConfig{
				InsecureSkipVerify: true,
				ServerName:         "test.com",
				CACert:             []byte(plainCACert),
				ClientCert:         []byte("-----BEGIN CERTIFICATE-----\nPLAIN_CLIENT_CERT\n-----END CERTIFICATE-----"),
				ClientKey:          []byte("-----BEGIN PRIVATE KEY-----\nPLAIN_CLIENT_KEY\n-----END PRIVATE KEY-----"),
			},
			wantCA: plainCACert,
		},
		"TLS with mixed secret and plain values": {
			tlsConfig: &sm.TLSConfig{
				InsecureSkipVerify: false,
				ServerName:         "mixed.com",
				CACert:             []byte("${secrets.ca-cert-key}"),
				ClientCert:         []byte("-----BEGIN CERTIFICATE-----\nPLAIN_CLIENT_CERT\n-----END CERTIFICATE-----"),
				ClientKey:          []byte("${secrets.client-key-key}"),
			},
			wantCA: secretCACert,
		},
		"TLS with only some fields": {
			tlsConfig: &sm.TLSConfig{
				InsecureSkipVerify: false,
				ServerName:         "partial.com",
				CACert:             []byte("${secrets.ca-cert-key}"),
				// ClientCert and ClientKey are empty
			},
			wantCA: secretCACert,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			resolver := interpolation.NewResolver(mockSecretStore, tenantID, logger)

			cfg, err := ResolveSMtoProm(ctx, logger, tc.tlsConfig, resolver)

			require.NoError(t, err)
			require.Equal(t, tc.tlsConfig.InsecureSkipVerify, cfg.InsecureSkipVerify)
			require.Equal(t, tc.tlsConfig.ServerName, cfg.ServerName)

			// The file holds the resolved value, not the reference.
			ca, err := os.ReadFile(cfg.CAFile)
			require.NoError(t, err)
			require.Equal(t, tc.wantCA, string(ca))

			if len(tc.tlsConfig.ClientCert) > 0 {
				require.NotEmpty(t, cfg.CertFile)
			}

			if len(tc.tlsConfig.ClientKey) > 0 {
				require.NotEmpty(t, cfg.KeyFile)
			}
		})
	}
}
