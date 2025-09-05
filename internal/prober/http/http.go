package http

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/interpolation"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
	"github.com/grafana/synthetic-monitoring-agent/internal/tls"
	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	bbeprober "github.com/prometheus/blackbox_exporter/prober"
	"github.com/prometheus/client_golang/prometheus"
	promconfig "github.com/prometheus/common/config"
	"github.com/rs/zerolog"
)

var errUnsupportedCheck = errors.New("unsupported check")

type Prober struct {
	// Raw settings and dependencies for runtime secret resolution
	settings                   *sm.HttpSettings
	timeout                    time.Duration
	secretStore                secrets.SecretProvider
	tenantID                   model.GlobalID
	logger                     zerolog.Logger
	cacheBustingQueryParamName string
	// Static config that doesn't need secret resolution
	staticConfig config.Module
}

func NewProber(ctx context.Context, check model.Check, logger zerolog.Logger, reservedHeaders http.Header, secretStore secrets.SecretProvider) (Prober, error) {
	if check.Settings.Http == nil {
		return Prober{}, errUnsupportedCheck
	}

	if len(reservedHeaders) > 0 {
		augmentHttpHeaders(&check.Check, reservedHeaders)
	}

	// Build static configuration (everything except authentication secrets)
	staticCfg, err := buildStaticConfig(check.Settings.Http)
	if err != nil {
		return Prober{}, err
	}

	staticCfg.Timeout = time.Duration(check.Timeout) * time.Millisecond

	return Prober{
		settings:                   check.Settings.Http,
		timeout:                    time.Duration(check.Timeout) * time.Millisecond,
		secretStore:                secretStore,
		tenantID:                   check.GlobalTenantID(),
		logger:                     logger.With().Str("prober", "http").Logger(),
		cacheBustingQueryParamName: check.Settings.Http.CacheBustingQueryParamName,
		staticConfig:               staticCfg,
	}, nil
}

func (p Prober) Name() string {
	return "http"
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger) (bool, float64) {
	slogger := logger.ToSlog(l)
	if p.cacheBustingQueryParamName != "" {
		// FIXME(mem): the second target argument should be the probe's name
		target = addCacheBustParam(target, p.cacheBustingQueryParamName, target)
	}

	// Resolve secrets and build complete config at probe time
	probeConfig, err := p.buildProbeConfig(ctx)
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to resolve secrets for HTTP probe")
		return false, 0
	}

	return bbeprober.ProbeHTTP(ctx, target, probeConfig, registry, slogger), 0
}

// buildProbeConfig creates the complete configuration with resolved secrets
func (p Prober) buildProbeConfig(ctx context.Context) (config.Module, error) {
	// Start with static config
	cfg := p.staticConfig

	// Resolve authentication secrets at probe time
	httpClientConfig, err := buildPrometheusHTTPClientConfig(
		ctx,
		p.settings,
		p.logger,
		p.secretStore,
		p.tenantID,
	)
	if err != nil {
		return cfg, fmt.Errorf("failed to build HTTP client config: %w", err)
	}

	cfg.HTTP.HTTPClientConfig = httpClientConfig

	// Set BBE's SkipResolvePhaseWithProxy when a proxy is configured
	if cfg.HTTP.HTTPClientConfig.ProxyURL.URL != nil {
		cfg.HTTP.SkipResolvePhaseWithProxy = true
	}

	// Handle OAuth2 config if present
	if p.settings.Oauth2Config != nil && p.settings.Oauth2Config.ClientId != "" {
		oauth2Config, err := convertOAuth2Config(ctx, p.settings.Oauth2Config, p.logger)
		if err != nil {
			return cfg, fmt.Errorf("parsing OAuth2 settings: %w", err)
		}
		cfg.HTTP.HTTPClientConfig.OAuth2 = oauth2Config
	}

	return cfg, nil
}

// buildStaticConfig creates the parts of the config that don't require secret resolution
func buildStaticConfig(settings *sm.HttpSettings) (config.Module, error) {
	var m config.Module

	m.Prober = sm.CheckTypeHttp.String()

	m.HTTP.IPProtocol, m.HTTP.IPProtocolFallback = settings.IpVersion.ToIpProtocol()

	m.HTTP.Body = settings.Body

	m.HTTP.Method = settings.Method.String()

	m.HTTP.FailIfSSL = settings.FailIfSSL

	m.HTTP.FailIfNotSSL = settings.FailIfNotSSL

	m.HTTP.Headers = buildHttpHeaders(settings.Headers)

	if settings.Compression != sm.CompressionAlgorithm_none {
		m.HTTP.Compression = settings.Compression.String()
	}

	m.HTTP.ValidStatusCodes = make([]int, 0, len(settings.ValidStatusCodes))
	for _, code := range settings.ValidStatusCodes {
		m.HTTP.ValidStatusCodes = append(m.HTTP.ValidStatusCodes, int(code))
	}

	m.HTTP.ValidHTTPVersions = make([]string, len(settings.ValidHTTPVersions))
	copy(m.HTTP.ValidHTTPVersions, settings.ValidHTTPVersions)

	m.HTTP.FailIfBodyMatchesRegexp = make([]config.Regexp, 0, len(settings.FailIfBodyMatchesRegexp))
	for _, str := range settings.FailIfBodyMatchesRegexp {
		re, err := config.NewRegexp(str)
		if err != nil {
			return m, err
		}

		m.HTTP.FailIfBodyMatchesRegexp = append(m.HTTP.FailIfBodyMatchesRegexp, re)
	}

	m.HTTP.FailIfBodyNotMatchesRegexp = make([]config.Regexp, 0, len(settings.FailIfBodyNotMatchesRegexp))
	for _, str := range settings.FailIfBodyNotMatchesRegexp {
		re, err := config.NewRegexp(str)
		if err != nil {
			return m, err
		}

		m.HTTP.FailIfBodyNotMatchesRegexp = append(m.HTTP.FailIfBodyNotMatchesRegexp, re)
	}

	m.HTTP.FailIfHeaderMatchesRegexp = make([]config.HeaderMatch, 0, len(settings.FailIfHeaderMatchesRegexp))
	for _, match := range settings.FailIfHeaderMatchesRegexp {
		re, err := config.NewRegexp(match.Regexp)
		if err != nil {
			return m, err
		}

		m.HTTP.FailIfHeaderMatchesRegexp = append(m.HTTP.FailIfHeaderMatchesRegexp, config.HeaderMatch{
			Header:       match.Header,
			Regexp:       re,
			AllowMissing: match.AllowMissing,
		})
	}

	m.HTTP.FailIfHeaderNotMatchesRegexp = make([]config.HeaderMatch, 0, len(settings.FailIfHeaderNotMatchesRegexp))
	for _, match := range settings.FailIfHeaderNotMatchesRegexp {
		re, err := config.NewRegexp(match.Regexp)
		if err != nil {
			return m, err
		}

		m.HTTP.FailIfHeaderNotMatchesRegexp = append(m.HTTP.FailIfHeaderNotMatchesRegexp, config.HeaderMatch{
			Header:       match.Header,
			Regexp:       re,
			AllowMissing: match.AllowMissing,
		})
	}

	return m, nil
}

// resolveSecretValue resolves a secret value using string interpolation with ${secrets.secret_name} syntax.
// If secretManagerEnabled is false, the value is returned as-is without any interpolation.
func resolveSecretValue(ctx context.Context, value string, secretStore secrets.SecretProvider, tenantID model.GlobalID, logger zerolog.Logger, secretManagerEnabled bool) (string, error) {
	if value == "" {
		return "", nil
	}

	// If secret manager is not enabled, return the value as-is
	if !secretManagerEnabled {
		return value, nil
	}

	// Create a resolver that only handles secrets (no variables)
	resolver := interpolation.NewResolver(nil, secretStore, tenantID, logger, secretManagerEnabled)
	return resolver.Resolve(ctx, value)
}

func buildPrometheusHTTPClientConfig(ctx context.Context, settings *sm.HttpSettings, logger zerolog.Logger, secretStore secrets.SecretProvider, tenantID model.GlobalID) (promconfig.HTTPClientConfig, error) {
	var cfg promconfig.HTTPClientConfig

	// Enable HTTP2 for all checks.
	cfg.EnableHTTP2 = true

	// We could do something like this instead:
	//
	// for _, v := range m.HTTP.ValidHTTPVersions {
	// 	if strings.HasPrefix(v, "HTTP/2") {
	// 		cfg.EnableHTTP2 = true
	// 		break
	// 	}
	// }
	//
	// but this needs to be evaluated. Go changed the behaviour so
	// that HTTP2 is enabled, and blacbox exporter follows that in
	// v0.21.0 (this setting defaults to true). We could add a
	// setting to _disable_ HTTP2. Eventually we are going to
	// introduce support for HTTP3, so that setting should be
	// something closer to what Go itself does which is specify a
	// supported / wanted protocol.

	cfg.FollowRedirects = !settings.NoFollowRedirects

	if settings.TlsConfig != nil {
		var err error
		cfg.TLSConfig, err = buildTLSConfig(ctx, settings.TlsConfig, secretStore, tenantID, logger, settings.SecretManagerEnabled)
		if err != nil {
			return cfg, err
		}
	}

	// Resolve bearer token (may be a secret)
	bearerToken, err := resolveSecretValue(ctx, settings.BearerToken, secretStore, tenantID, logger, settings.SecretManagerEnabled)
	if err != nil {
		return cfg, fmt.Errorf("failed to resolve bearer token: %w", err)
	}
	cfg.BearerToken = promconfig.Secret(bearerToken)

	if settings.BasicAuth != nil {
		// Resolve password (may be a secret)
		password, err := resolveSecretValue(ctx, settings.BasicAuth.Password, secretStore, tenantID, logger, settings.SecretManagerEnabled)
		if err != nil {
			return cfg, fmt.Errorf("failed to resolve basic auth password: %w", err)
		}

		cfg.BasicAuth = &promconfig.BasicAuth{
			Username: settings.BasicAuth.Username,
			Password: promconfig.Secret(password),
		}
	}

	if settings.ProxyURL != "" {
		var err error
		cfg.ProxyURL.URL, err = url.Parse(settings.ProxyURL)
		if err != nil {
			return cfg, fmt.Errorf("parsing proxy URL: %w", err)
		}

		if len(settings.ProxyConnectHeaders) > 0 {
			headers := make(promconfig.ProxyHeader)
			for _, h := range settings.ProxyConnectHeaders {
				name, value := strToHeaderNameValue(h)
				headers[name] = []promconfig.Secret{promconfig.Secret(value)}
			}
			cfg.ProxyConnectHeader = headers
		}
	}

	return cfg, nil
}

// buildTLSConfig builds a Prometheus TLS config from SM TLS config with secret resolution support
func buildTLSConfig(ctx context.Context, tlsConfig *sm.TLSConfig, secretStore secrets.SecretProvider, tenantID model.GlobalID, logger zerolog.Logger, secretManagerEnabled bool) (promconfig.TLSConfig, error) {
	// Create a copy of the TLS config with resolved secrets
	resolvedTLSConfig := &sm.TLSConfig{
		InsecureSkipVerify: tlsConfig.InsecureSkipVerify,
		ServerName:         tlsConfig.ServerName,
	}

	// Resolve CA cert if present
	if len(tlsConfig.CACert) > 0 {
		if secretManagerEnabled {
			// Resolve CA cert from secret if secret manager is enabled
			caCertStr, err := resolveSecretValue(ctx, string(tlsConfig.CACert), secretStore, tenantID, logger, secretManagerEnabled)
			if err != nil {
				return promconfig.TLSConfig{}, fmt.Errorf("failed to resolve CA cert: %w", err)
			}
			resolvedTLSConfig.CACert = []byte(caCertStr)
		} else {
			resolvedTLSConfig.CACert = tlsConfig.CACert
		}
	}

	// Resolve client cert if present
	if len(tlsConfig.ClientCert) > 0 {
		if secretManagerEnabled {
			// Resolve client cert from secret if secret manager is enabled
			clientCertStr, err := resolveSecretValue(ctx, string(tlsConfig.ClientCert), secretStore, tenantID, logger, secretManagerEnabled)
			if err != nil {
				return promconfig.TLSConfig{}, fmt.Errorf("failed to resolve client cert: %w", err)
			}
			resolvedTLSConfig.ClientCert = []byte(clientCertStr)
		} else {
			resolvedTLSConfig.ClientCert = tlsConfig.ClientCert
		}
	}

	// Resolve client key if present
	if len(tlsConfig.ClientKey) > 0 {
		if secretManagerEnabled {
			// Resolve client key from secret if secret manager is enabled
			clientKeyStr, err := resolveSecretValue(ctx, string(tlsConfig.ClientKey), secretStore, tenantID, logger, secretManagerEnabled)
			if err != nil {
				return promconfig.TLSConfig{}, fmt.Errorf("failed to resolve client key: %w", err)
			}
			resolvedTLSConfig.ClientKey = []byte(clientKeyStr)
		} else {
			resolvedTLSConfig.ClientKey = tlsConfig.ClientKey
		}
	}

	// Use the existing TLS conversion function with resolved config
	return tls.SMtoProm(ctx, logger, resolvedTLSConfig)
}

func convertOAuth2Config(ctx context.Context, cfg *sm.OAuth2Config, logger zerolog.Logger) (*promconfig.OAuth2, error) {
	r := &promconfig.OAuth2{}
	r.ClientID = cfg.ClientId
	r.ClientSecret = promconfig.Secret(cfg.ClientSecret)
	r.TokenURL = cfg.TokenURL
	r.Scopes = make([]string, len(cfg.Scopes))
	copy(r.Scopes, cfg.Scopes)
	r.EndpointParams = make(map[string]string, len(cfg.EndpointParams))
	for _, pair := range cfg.EndpointParams {
		r.EndpointParams[pair.Name] = pair.Value
	}
	var err error
	if cfg.ProxyURL != "" {
		r.ProxyURL.URL, err = url.Parse(cfg.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("parsing proxy URL: %w", err)
		}
	}
	if cfg.TlsConfig != nil {
		r.TLSConfig, err = tls.SMtoProm(ctx, logger, cfg.TlsConfig)
		if err != nil {
			return nil, fmt.Errorf("parsing TLS config: %w", err)
		}
	}
	return r, nil
}

// Overrides any user-provided headers with our own augmented values
// for reserved headers.
func augmentHttpHeaders(check *sm.Check, reservedHeaders http.Header) {
	headers := []string{}
	for _, header := range check.Settings.Http.Headers {
		name, _ := strToHeaderNameValue(header)

		_, present := reservedHeaders[http.CanonicalHeaderKey(name)]
		if present {
			continue // users can't override reserved headers with their own values
		}

		headers = append(headers, header)
	}

	for key, values := range reservedHeaders {
		var b strings.Builder
		for _, value := range values {
			b.Reset()
			b.WriteString(key)
			b.WriteRune(':')
			b.WriteString(value)
			headers = append(headers, b.String())
		}
	}

	check.Settings.Http.Headers = headers
}

func buildHttpHeaders(headers []string) map[string]string {
	userAgentHeader := "user-agent"

	h := map[string]string{
		userAgentHeader: version.UserAgent(), // default user-agent header
	}

	for _, header := range headers {
		name, value := strToHeaderNameValue(header)

		if strings.ToLower(name) == userAgentHeader {
			// Remove the default user-agent header and
			// replace it with the one the user is
			// specifying, so that we respect whatever case
			// they chose (e.g. "user-agent" vs
			// "User-Agent").
			delete(h, userAgentHeader)
		}

		h[name] = value
	}

	return h
}

func strToHeaderNameValue(s string) (string, string) {
	parts := strings.SplitN(s, ":", 2)

	var value string
	if len(parts) == 2 {
		value = strings.TrimLeft(parts[1], " ")
	}

	return parts[0], value
}

func addCacheBustParam(target, paramName, salt string) string {
	// we already know this URL is valid
	u, _ := url.Parse(target)
	q := u.Query()
	value := hashString(salt, strconv.FormatInt(time.Now().UnixNano(), 10))
	q.Set(paramName, value)
	u.RawQuery = q.Encode()
	return u.String()
}

func hashString(salt, str string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(salt))
	_, _ = h.Write([]byte(str))
	return strconv.FormatUint(h.Sum64(), 16)
}
