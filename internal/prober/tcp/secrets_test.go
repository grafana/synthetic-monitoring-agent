package tcp

import (
	"bytes"
	"context"
	"crypto/tls"
	"regexp"
	"strings"
	"testing"

	kitlog "github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// TestSecretsOnTheWire checks what the server receives and what both logs show. The echo server
// answers every line with "PONG <line>", so each sent secret also comes back in a read line.
func TestSecretsOnTheWire(t *testing.T) {
	certPEM, keyPEM, serverTLS := testhelper.MutualTLS(t)

	values := map[string]string{
		"user":   "user-7f3a",
		"pass":   "pass-9c2e",
		"dollar": "pa$$w$1rd",
		// Starts with letters, digits and _, which regexp.Expand reads as part of a $1 before it.
		"word":  "word_8e1f",
		"lf":    "lf-5d1b\n",
		"crlf":  "crlf-2a7c\r\n",
		"lines": "lines-a\nlines-b",
		"ca":    string(certPEM),
		"cert":  string(certPEM),
		"key":   string(keyPEM),
	}

	// A piece of the key body: zerolog escapes the newlines, so the whole PEM would never match.
	keyBody := string(bytes.Split(keyPEM, []byte("\n"))[1])

	secretStore := testhelper.NewMockSecretProvider(values)

	type testcase struct {
		banner       string
		serverTLS    *tls.Config
		settings     sm.TcpSettings
		store        secrets.SecretProvider
		wantSuccess  bool
		wantGot      []string
		wantCheckLog []string
	}

	testcases := map[string]testcase{
		"mixed text and two references": {
			settings: sm.TcpSettings{QueryResponse: []sm.TCPQueryResponse{
				{Send: []byte("AUTH ${secrets.user} ${secrets.pass}")},
				{Expect: []byte("^PONG")},
			}},
			wantSuccess: true,
			wantGot:     []string{"AUTH user-7f3a pass-9c2e"},
			// The echoed line.
			wantCheckLog: []string{"PONG AUTH ${secrets.user} ${secrets.pass}"},
		},

		// Blackbox exporter expands $ only in a step with an expected reply.
		"$ in a secret, with and without an expected reply": {
			banner: "PONG",
			settings: sm.TcpSettings{QueryResponse: []sm.TCPQueryResponse{
				{Expect: []byte("^(PONG)"), Send: []byte("$1 ${secrets.dollar}")},
				{Send: []byte("${secrets.dollar}")},
				{Expect: []byte("^PONG")},
			}},
			wantSuccess:  true,
			wantGot:      []string{"PONG pa$$w$1rd", "pa$$w$1rd"},
			wantCheckLog: []string{"PONG ${secrets.dollar}"},
		},

		"$1 and a lone $ right before a reference": {
			banner: "NONCE abc123",
			settings: sm.TcpSettings{QueryResponse: []sm.TCPQueryResponse{
				{Expect: []byte(`^NONCE (\w+)`), Send: []byte("AUTH $1${secrets.word} $${secrets.word} $$1${secrets.word}")},
				{Expect: []byte("^PONG")},
			}},
			wantSuccess:  true,
			wantGot:      []string{"AUTH abc123word_8e1f $word_8e1f $1word_8e1f"},
			wantCheckLog: []string{"PONG AUTH abc123${secrets.word} $${secrets.word} $1${secrets.word}"},
		},

		"missing secret": {
			settings: sm.TcpSettings{QueryResponse: []sm.TCPQueryResponse{
				{Send: []byte("AUTH ${secrets.pass}")},
				{Send: []byte("${secrets.missing}")},
			}},
			wantCheckLog: []string{`field="sent text"`, "secret=missing", secrets.ErrSecretNotFound.Error()},
		},

		"no references": {
			settings: sm.TcpSettings{QueryResponse: []sm.TCPQueryResponse{
				{Send: []byte("PING")},
				{Expect: []byte("^PONG")},
			}},
			store: testhelper.NewMockSecretProviderWithFunc(func(context.Context, model.GlobalID, string) (string, error) {
				t.Error("a check without references called the secret store")

				return "", nil
			}),
			wantSuccess: true,
			wantGot:     []string{"PING"},
		},

		// The server requires a client certificate, so the run succeeds only if the CA cert,
		// client cert and client key all resolve.
		"TLS material from secrets": {
			serverTLS: serverTLS,
			settings: sm.TcpSettings{
				Tls: true,
				TlsConfig: &sm.TLSConfig{
					CACert:     []byte("${secrets.ca}"),
					ClientCert: []byte("${secrets.cert}"),
					ClientKey:  []byte("${secrets.key}"),
				},
				QueryResponse: []sm.TCPQueryResponse{
					{Send: []byte("PING")},
					{Expect: []byte("^PONG")},
				},
			},
			wantSuccess: true,
			wantGot:     []string{"PING"},
		},

		// The TLS material holds no reference, so the run uses the files built with the prober.
		"secret in sent text, TLS material inline": {
			serverTLS: serverTLS,
			settings: sm.TcpSettings{
				Tls: true,
				TlsConfig: &sm.TLSConfig{
					CACert:     certPEM,
					ClientCert: certPEM,
					ClientKey:  keyPEM,
				},
				QueryResponse: []sm.TCPQueryResponse{
					{Send: []byte("AUTH ${secrets.user}")},
					{Expect: []byte("^PONG")},
				},
			},
			wantSuccess: true,
			wantGot:     []string{"AUTH user-7f3a"},
		},
	}

	// The server would echo the secret back, and blackbox exporter logs each line it reads with
	// the line break removed, which no redaction of the stored value matches.
	for _, secret := range []string{"lf", "crlf", "lines"} {
		testcases["line break in secret "+secret] = testcase{
			settings: sm.TcpSettings{QueryResponse: []sm.TCPQueryResponse{
				{Send: []byte("AUTH ${secrets.user}")},
				{Send: []byte("AUTH ${secrets." + secret + "}")},
				{Expect: []byte("^PONG")},
			}},
			wantCheckLog: []string{`field="sent text"`, "secret=" + secret, secrets.ErrSecretValueInvalidInSentText.Error()},
		}
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			ctx := testCtx(context.Background(), t)

			addr, got := runEchoServer(t, tc.banner, tc.serverTLS)

			store := tc.store
			if store == nil {
				store = secretStore
			}

			check := model.Check{Check: sm.Check{
				Target:   addr,
				Timeout:  5000,
				Settings: sm.CheckSettings{Tcp: &tc.settings},
			}}

			var (
				agentLog testhelper.LogBuffer
				checkLog bytes.Buffer
			)

			prober, err := NewProber(ctx, check, zerolog.New(&agentLog), store)
			require.NoError(t, err)

			success, _ := prober.Probe(ctx, addr, prometheus.NewRegistry(), kitlog.NewLogfmtLogger(&checkLog), "")
			require.Equal(t, tc.wantSuccess, success, checkLog.String())
			require.Equal(t, tc.wantGot, got())

			for _, want := range tc.wantCheckLog {
				require.Contains(t, checkLog.String(), want)
			}

			for _, log := range []string{checkLog.String(), agentLog.String()} {
				for _, name := range []string{"user", "pass", "dollar", "word", "lf", "crlf", "lines"} {
					// Line by line, as a value with a line break never appears whole in a log line.
					for _, line := range strings.FieldsFunc(values[name], func(r rune) bool { return r == '\r' || r == '\n' }) {
						require.NotContains(t, log, line)
					}
				}

				require.NotContains(t, log, keyBody)
			}
		})
	}
}

// TestCloseDollar checks that regexp.Expand reads the text before a reference the same way
// whatever value follows it. Values arrive with each $ doubled.
func TestCloseDollar(t *testing.T) {
	re := regexp.MustCompile(`(?P<name>x)`)
	match := re.FindStringSubmatchIndex("x")

	expand := func(template string) string {
		return string(re.ExpandString(nil, template, "x", match))
	}

	gaps := []string{"", "AUTH ", "$1", "$name", "$é", "$", "$$", "$$1", "$$$1", "${", "${1", "${1}", "$1 ", "a$b$1", "$1$$"}
	values := []string{"abc", "1}", "{1}", "_x", "é", "$$x", ""}

	for _, gap := range gaps {
		for _, value := range values {
			require.Equal(t, expand(gap)+expand(value), expand(closeDollar(gap)+value), "gap %q, value %q", gap, value)
		}
	}
}
