package interpolation

import (
	"context"
	"crypto/rand"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/rs/zerolog"
)

type mapSecrets map[string]string

func (m mapSecrets) GetSecretValue(_ context.Context, _ model.GlobalID, key string) (string, error) {
	return m[key], nil
}

// pemOfSize returns a PEM block around derLen random bytes. 790 and 1218 match an RSA-2048
// self-signed cert and its PKCS#8 key.
func pemOfSize(b *testing.B, typ string, derLen int) string {
	der := make([]byte, derLen)
	if _, err := rand.Read(der); err != nil {
		b.Fatal(err)
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}))
}

var sink string

func BenchmarkResolve(b *testing.B) {
	cert := pemOfSize(b, "CERTIFICATE", 790)

	inputs := []struct{ name, value string }{
		{"password-28B", "correct-horse-battery-staple"},
		{"token-64B", "glc_" + strings.Repeat("x", 60)},
		{"cert-1.1K", cert},
		{"key-1.7K", pemOfSize(b, "PRIVATE KEY", 1218)},
		{"bundle-8K", strings.Repeat(cert, 8)},
		{"ref-whole", "${secrets.api-token}"},
		{"ref-in-bundle-8K", strings.Repeat(cert, 8) + "${secrets.ca-crt}"},
	}

	secrets := mapSecrets{"api-token": "glc_" + strings.Repeat("y", 60), "ca-crt": cert}
	resolver := NewResolver(secrets, 0, zerolog.Nop())
	ctx := context.Background()

	for _, in := range inputs {
		b.Run(in.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(in.value)))

			for b.Loop() {
				out, err := resolver.Resolve(ctx, in.value)
				if err != nil {
					b.Fatal(err)
				}

				sink = out
			}
		})
	}
}
