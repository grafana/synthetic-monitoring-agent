package http

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
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
	config                     config.Module
	cacheBustingQueryParamName string
}

func NewProber(ctx context.Context, check sm.Check, logger zerolog.Logger) (Prober, error) {
	if check.Settings.Http == nil {
		return Prober{}, errUnsupportedCheck
	}

	cfg, err := settingsToModule(ctx, check.Settings.Http, logger)
	if err != nil {
		return Prober{}, err
	}

	cfg.Timeout = time.Duration(check.Timeout) * time.Millisecond

	return Prober{
		config:                     cfg,
		cacheBustingQueryParamName: check.Settings.Http.CacheBustingQueryParamName,
	}, nil
}

func (p Prober) Name() string {
	return "http"
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger) bool {
	if p.cacheBustingQueryParamName != "" {
		// FIXME(mem): the second target argument should be the probe's name
		target = addCacheBustParam(target, p.cacheBustingQueryParamName, target)
	}

	return bbeprober.ProbeHTTP(ctx, target, p.config, registry, logger)
}

func settingsToModule(ctx context.Context, settings *sm.HttpSettings, logger zerolog.Logger) (config.Module, error) {
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

	var err error
	m.HTTP.HTTPClientConfig, err = buildPrometheusHTTPClientConfig(
		ctx,
		settings,
		logger.With().Str("prober", m.Prober).Logger(),
	)
	if err != nil {
		return m, err
	}

	// Set BBE's SkipResolvePhaseWithProxy when a proxy is configured to avoid resolving the target.
	// DNS should be done at the proxy server only.
	if m.HTTP.HTTPClientConfig.ProxyURL.URL != nil {
		m.HTTP.SkipResolvePhaseWithProxy = true
	}

	if settings.Oauth2Config != nil && settings.Oauth2Config.ClientId != "" {
		var err error
		m.HTTP.HTTPClientConfig.OAuth2, err = convertOAuth2Config(ctx, settings.Oauth2Config, logger.With().Str("prober", m.Prober).Logger())
		if err != nil {
			return m, fmt.Errorf("parsing OAuth2 settings: %w", err)
		}
	}

	return m, nil
}

func buildPrometheusHTTPClientConfig(ctx context.Context, settings *sm.HttpSettings, logger zerolog.Logger) (promconfig.HTTPClientConfig, error) {
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
		cfg.TLSConfig, err = tls.SMtoProm(ctx, logger, settings.TlsConfig)
		if err != nil {
			return cfg, err
		}
	}

	cfg.BearerToken = promconfig.Secret(settings.BearerToken)

	if settings.BasicAuth != nil {
		cfg.BasicAuth = &promconfig.BasicAuth{
			Username: settings.BasicAuth.Username,
			Password: promconfig.Secret(settings.BasicAuth.Password),
		}
	}

	if settings.ProxyURL != "" {
		var err error
		cfg.ProxyURL.URL, err = url.Parse(settings.ProxyURL)
		if err != nil {
			return cfg, fmt.Errorf("parsing proxy URL: %w", err)
		}

		if len(settings.ProxyConnectHeaders) > 0 {
			headers := make(promconfig.Header)
			for _, h := range settings.ProxyConnectHeaders {
				name, value := strToHeaderNameValue(h)
				headers[name] = []promconfig.Secret{promconfig.Secret(value)}
			}
			cfg.ProxyConnectHeader = headers
		}
	}

	return cfg, nil
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
