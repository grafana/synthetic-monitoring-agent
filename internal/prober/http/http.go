package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log/level"
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
	"golang.org/x/net/http/httpguts"
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
	// Built once; header values still hold ${secrets.name}, resolved on each run.
	staticConfig config.Module
}

func NewProber(ctx context.Context, check model.Check, logger zerolog.Logger, reservedHeaders http.Header, secretStore secrets.SecretProvider) (Prober, error) {
	if check.Settings.Http == nil {
		return Prober{}, errUnsupportedCheck
	}

	if len(reservedHeaders) > 0 {
		augmentHttpHeaders(&check.Check, reservedHeaders)
	}

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

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger, _ string) (bool, float64) {
	if p.cacheBustingQueryParamName != "" {
		// FIXME(mem): the second target argument should be the probe's name
		target = addCacheBustParam(target, p.cacheBustingQueryParamName, target)
	}

	resolver := interpolation.NewResolver(p.secretStore, p.tenantID, p.logger)

	// Resolve secrets and build complete config at probe time
	probeConfig, err := p.buildProbeConfig(ctx, resolver)

	// Blackbox exporter logs the Host header and the redirects it follows, and a target can echo
	// a value back, so the check logs get each resolved value as its reference.
	l = logger.Redacting(l, resolver.Redactor())

	if err != nil {
		p.logger.Error().Err(err).Msg("failed to build config for HTTP probe")

		// Only a secret failure reaches the check logs.
		if secretErr, ok := errors.AsType[*secrets.Error](err); ok {
			msg := "Could not resolve a secret referenced by this check"
			if errors.Is(secretErr, secrets.ErrSecretValueInvalidInHeader) {
				msg = "Could not send a secret referenced by this check"
			}

			_ = level.Error(l).Log(append([]any{"msg", msg}, secretErr.LogFields()...)...)
		}

		return false, 0
	}

	return bbeprober.ProbeHTTP(ctx, target, probeConfig, registry, logger.ToSlog(l)), 0
}

// buildProbeConfig creates the complete configuration with resolved secrets
func (p Prober) buildProbeConfig(ctx context.Context, resolver *interpolation.Resolver) (config.Module, error) {
	// Start with static config
	cfg := p.staticConfig

	httpClientConfig, err := buildPrometheusHTTPClientConfig(ctx, p.settings, p.logger, resolver)
	if err != nil {
		return cfg, fmt.Errorf("failed to build HTTP client config: %w", err)
	}

	cfg.HTTP.HTTPClientConfig = httpClientConfig

	cfg.HTTP.Headers, err = resolveHeaders(ctx, p.staticConfig.HTTP.Headers, resolver)
	if err != nil {
		return cfg, err
	}

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

// buildStaticConfig builds the parts of the config that NewProber can build once. Header values
// can still hold secret references, which buildProbeConfig resolves on each run.
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

// resolveSecretValue resolves the secret references in value. field names the setting in the
// check logs when a reference does not resolve.
func resolveSecretValue(ctx context.Context, field, value string, resolver *interpolation.Resolver) (string, error) {
	resolved, err := resolver.Resolve(ctx, value)
	if err != nil {
		secretErr := secrets.AsError(err)
		secretErr.Field = field

		return "", secretErr
	}

	return resolved, nil
}

// resolveSecretBytes is resolveSecretValue for []byte. A value with no reference is returned as
// is, without converting it to a string and back.
func resolveSecretBytes(ctx context.Context, field string, value []byte, resolver *interpolation.Resolver) ([]byte, error) {
	if !bytes.Contains(value, []byte(interpolation.SecretRefPrefix)) {
		return value, nil
	}

	resolved, err := resolveSecretValue(ctx, field, string(value), resolver)
	if err != nil {
		return nil, err
	}

	return []byte(resolved), nil
}

// resolveHeaders returns the headers with each secret reference in a value resolved. Names are never resolved.
func resolveHeaders(ctx context.Context, headers map[string]string, resolver *interpolation.Resolver) (map[string]string, error) {
	if !anyValueHasSecretRef(headers) {
		return headers, nil
	}

	resolved := make(map[string]string, len(headers))

	// Sorted, so that when several headers fail the check logs always report the same one.
	for _, name := range slices.Sorted(maps.Keys(headers)) {
		field := "header " + name

		v, err := resolveSecretValue(ctx, field, headers[name], resolver)
		if err != nil {
			return nil, err
		}

		if err := checkResolvedHeaderValue(field, headers[name], v); err != nil {
			return nil, err
		}

		resolved[name] = v
	}

	return resolved, nil
}

// checkResolvedHeaderValue fails when resolving put a character into a header value that HTTP does
// not allow, such as the line break a stored secret often ends with. The HTTP client rejects such a
// request too, but its error does not tell the check owner that a secret is the cause. A bad
// character typed into the check itself is left to the HTTP client.
func checkResolvedHeaderValue(field, raw, resolved string) error {
	if httpguts.ValidHeaderFieldValue(resolved) || !httpguts.ValidHeaderFieldValue(raw) {
		return nil
	}

	err := &secrets.Error{Field: field, Reason: secrets.ErrSecretValueInvalidInHeader}

	// With more than one reference, the value does not tell which secret is the cause.
	if refs := interpolation.SecretRegex.FindAllStringSubmatch(raw, -1); len(refs) == 1 {
		err.Secret = refs[0][1]
	}

	return err
}

func anyValueHasSecretRef(headers map[string]string) bool {
	for _, value := range headers {
		if interpolation.HasSecretRef(value) {
			return true
		}
	}

	return false
}

func buildPrometheusHTTPClientConfig(ctx context.Context, settings *sm.HttpSettings, logger zerolog.Logger, resolver *interpolation.Resolver) (promconfig.HTTPClientConfig, error) {
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

		cfg.TLSConfig, err = buildTLSConfig(ctx, settings.TlsConfig, logger, resolver)
		if err != nil {
			return cfg, err
		}
	}

	// Resolve bearer token (may be a secret)
	bearerToken, err := resolveSecretValue(ctx, "bearer token", settings.BearerToken, resolver)
	if err != nil {
		return cfg, err
	}

	cfg.BearerToken = promconfig.Secret(bearerToken)

	if settings.BasicAuth != nil {
		// Resolve password (may be a secret)
		password, err := resolveSecretValue(ctx, "basic auth password", settings.BasicAuth.Password, resolver)
		if err != nil {
			return cfg, err
		}

		cfg.BasicAuth = &promconfig.BasicAuth{
			Username: settings.BasicAuth.Username,
			Password: promconfig.Secret(password),
		}
	}

	if settings.ProxyURL != "" {
		var err error

		cfg.ProxyURL.URL, err = parseProxyURL(settings.ProxyURL)
		if err != nil {
			return cfg, err
		}

		if len(settings.ProxyConnectHeaders) > 0 {
			headers := make(promconfig.ProxyHeader)

			for _, h := range settings.ProxyConnectHeaders {
				name, value := sm.SplitHeader(h)
				headers[name] = []promconfig.Secret{promconfig.Secret(value)}
			}

			cfg.ProxyConnectHeader = headers
		}
	}

	return cfg, nil
}

// buildTLSConfig builds a Prometheus TLS config from SM TLS config with secret resolution support
func buildTLSConfig(ctx context.Context, tlsConfig *sm.TLSConfig, logger zerolog.Logger, resolver *interpolation.Resolver) (promconfig.TLSConfig, error) {
	// Create a copy of the TLS config with resolved secrets
	resolvedTLSConfig := &sm.TLSConfig{
		InsecureSkipVerify: tlsConfig.InsecureSkipVerify,
		ServerName:         tlsConfig.ServerName,
	}

	var err error

	resolvedTLSConfig.CACert, err = resolveSecretBytes(ctx, "CA cert", tlsConfig.CACert, resolver)
	if err != nil {
		return promconfig.TLSConfig{}, err
	}

	resolvedTLSConfig.ClientCert, err = resolveSecretBytes(ctx, "client cert", tlsConfig.ClientCert, resolver)
	if err != nil {
		return promconfig.TLSConfig{}, err
	}

	resolvedTLSConfig.ClientKey, err = resolveSecretBytes(ctx, "client key", tlsConfig.ClientKey, resolver)
	if err != nil {
		return promconfig.TLSConfig{}, err
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
		r.ProxyURL.URL, err = parseProxyURL(cfg.ProxyURL)
		if err != nil {
			return nil, err
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

// parseProxyURL is url.Parse without the URL in its error.
func parseProxyURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		// url.Parse quotes the whole raw URL, and a proxy URL might have user:password@.
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			err = urlErr.Err
		}

		return nil, fmt.Errorf("parsing proxy URL: %w", err)
	}

	return u, nil
}

// Overrides any user-provided headers with our own augmented values
// for reserved headers.
func augmentHttpHeaders(check *sm.Check, reservedHeaders http.Header) {
	headers := []string{}

	for _, header := range check.Settings.Http.Headers {
		name, _ := sm.SplitHeader(header)

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
		name, value := sm.SplitHeader(header)

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
