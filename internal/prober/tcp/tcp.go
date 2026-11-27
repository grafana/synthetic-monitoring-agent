package tcp

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/interpolation"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
	"github.com/grafana/synthetic-monitoring-agent/internal/tls"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	bbeprober "github.com/prometheus/blackbox_exporter/prober"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

var errUnsupportedCheck = errors.New("unsupported check")

type Prober struct {
	config config.Module
	// settings is set only when the check references a secret, which then resolves on every run.
	settings    *sm.TcpSettings
	secretStore secrets.SecretProvider
	tenantID    model.GlobalID
	logger      zerolog.Logger
}

func NewProber(ctx context.Context, check model.Check, logger zerolog.Logger, secretStore secrets.SecretProvider) (Prober, error) {
	if check.Settings.Tcp == nil {
		return Prober{}, errUnsupportedCheck
	}

	cfg, err := settingsToModule(ctx, check.Settings.Tcp, logger)
	if err != nil {
		return Prober{}, err
	}

	cfg.Timeout = time.Duration(check.Timeout) * time.Millisecond

	p := Prober{config: cfg}

	if check.ReferencesSecrets() {
		p.settings = check.Settings.Tcp
		p.secretStore = secretStore
		p.tenantID = check.GlobalTenantID()
		p.logger = logger.With().Str("prober", p.Name()).Logger()
	}

	return p, nil
}

func (p Prober) Name() string {
	return "tcp"
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	cfg := p.config

	if p.settings != nil {
		resolver := interpolation.NewResolver(p.secretStore, p.tenantID, p.logger)

		var err error

		cfg, err = p.resolveSecrets(ctx, resolver)

		// Blackbox exporter logs every line it sends and reads, and a server can echo a secret back.
		l = logger.Redacting(l, resolver.Redactor())

		if err != nil {
			p.logger.Error().Err(err).Msg("failed to resolve secrets for TCP probe")
			interpolation.LogSecretError(l, err)

			return false, 0
		}
	}

	return bbeprober.ProbeTCP(ctx, target, cfg, registry, logger.ToSlog(l)), 0
}

// resolveSecrets returns a copy of the static config with each secret reference resolved, in
// the sent text of every step and in the TLS config.
func (p Prober) resolveSecrets(ctx context.Context, resolver *interpolation.Resolver) (config.Module, error) {
	cfg := p.config
	cfg.TCP.QueryResponse = slices.Clone(cfg.TCP.QueryResponse)

	for i := range cfg.TCP.QueryResponse {
		qr := &cfg.TCP.QueryResponse[i]

		// With an expected reply, blackbox exporter expands $1 in the sent text, as regexp.Expand
		// does. Closing a $ before a reference and escaping $ in values sends secrets as stored.
		expands := qr.Expect.Regexp != nil

		send := qr.Send
		if expands {
			send = closeDollarsBeforeRefs(send)
		}

		send, err := resolver.ResolveFunc(ctx, "sent text", send, func(v string) (string, error) {
			// A line break would split the sent line, and an echoed secret, logged without its
			// line break, would slip past the redactor.
			if strings.ContainsAny(v, "\r\n") {
				return "", &secrets.Error{Reason: secrets.ErrSecretValueInvalidInSentText}
			}

			if expands {
				v = strings.ReplaceAll(v, "$", "$$")
			}

			return v, nil
		})
		if err != nil {
			return cfg, err
		}

		qr.Send = send
	}

	// Without a reference, the TLS files built with the prober serve every run.
	if p.settings.TlsConfig.ReferencesSecrets() {
		var err error

		cfg.TCP.TLSConfig, err = tls.ResolveSMtoProm(ctx, p.logger, p.settings.TlsConfig, resolver)
		if err != nil {
			return cfg, err
		}
	}

	return cfg, nil
}

// closeDollarsBeforeRefs applies closeDollar to the text before each secret reference in send.
func closeDollarsBeforeRefs(send string) string {
	var b strings.Builder

	last := 0

	for _, ref := range interpolation.SecretRegex.FindAllStringIndex(send, -1) {
		b.WriteString(closeDollar(send[last:ref[0]]))
		b.WriteString(send[ref[0]:ref[1]])

		last = ref[1]
	}

	b.WriteString(send[last:])

	return b.String()
}

// openDollar matches an unpaired $ at the end of the text, which regexp.Expand would read on into
// what follows. Group 1 is the $ to the end, group 2 an optional {, group 3 the name so far.
var openDollar = regexp.MustCompile(`(?:^|[^$])(?:\$\$)*(\$(\{?)([\p{L}\p{Nd}_]*))$`)

// closeDollar ends an open $ at the end of s, so that regexp.Expand stops there whatever follows:
// $1 becomes ${1}, and any other open $ is doubled into a literal $.
func closeDollar(s string) string {
	m := openDollar.FindStringSubmatch(s)
	if m == nil {
		return s
	}

	open, brace, name := m[1], m[2], m[3]
	head := s[:len(s)-len(open)]

	if brace == "" && name != "" {
		return head + "${" + name + "}"
	}

	return head + "$" + open
}

func settingsToModule(ctx context.Context, settings *sm.TcpSettings, logger zerolog.Logger) (config.Module, error) {
	var m config.Module

	m.Prober = sm.CheckTypeTcp.String()

	m.TCP.IPProtocol, m.TCP.IPProtocolFallback = settings.IpVersion.ToIpProtocol()

	m.TCP.SourceIPAddress = settings.SourceIpAddress

	m.TCP.TLS = settings.Tls

	m.TCP.QueryResponse = make([]config.QueryResponse, 0, len(settings.QueryResponse))

	for _, qr := range settings.QueryResponse {
		entry := config.QueryResponse{
			Send:     string(qr.Send),
			StartTLS: qr.StartTLS,
		}

		// An empty expect has to stay a nil regexp: BBE reads a line for
		// every step that carries one, and an empty pattern matches anything.
		if len(qr.Expect) > 0 {
			re, err := config.NewRegexp(string(qr.Expect))
			if err != nil {
				return m, err
			}

			entry.Expect = re
		}

		m.TCP.QueryResponse = append(m.TCP.QueryResponse, entry)
	}

	if settings.TlsConfig != nil {
		var err error

		m.TCP.TLSConfig, err = tls.SMtoProm(ctx, logger.With().Str("prober", m.Prober).Logger(), settings.TlsConfig)
		if err != nil {
			return m, err
		}
	}

	return m, nil
}
