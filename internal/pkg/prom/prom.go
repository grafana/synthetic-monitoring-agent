package prom

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/mwitkow/go-conntrack"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/prompb"
)

const maxErrMsgLen = 256

var (
	maxRetries = 10
	minBackoff = 30 * time.Millisecond
	maxBackoff = 250 * time.Millisecond
)

type recoverableError struct {
	error
}

func NewRecoverableError(cause error) error {
	return recoverableError{cause}
}

type HttpError struct {
	StatusCode int
	Status     string
	Err        error
}

func (e *HttpError) Unwrap() error {
	return e.Err
}

func (e *HttpError) Error() string {
	return e.Err.Error()
}

func GetHttpStatusCode(err error) (int, bool) {
	for ; err != nil; err = errors.Unwrap(err) {
		err2, ok := err.(*HttpError)
		if ok {
			return err2.StatusCode, true
		}
	}

	return 0, false
}

type PrometheusClient interface {
	StoreBytes(ctx context.Context, req []byte) error
	StoreStream(ctx context.Context, req io.Reader) error
	CountRetries(retries float64)
}

func SendBytesWithBackoff(ctx context.Context, client PrometheusClient, req []byte) error {
	return withBackoff(ctx, client, func(ctx context.Context, client PrometheusClient) error {
		return client.StoreBytes(ctx, req)
	})
}

func withBackoff(ctx context.Context, client PrometheusClient, store func(ctx context.Context, prometheusClient PrometheusClient) error) error {
	clampBackoff := func(a time.Duration) time.Duration {
		if a > maxBackoff {
			return maxBackoff
		}
		return a
	}

	var err error

	backoff := minBackoff

	retries := maxRetries
	defer func() {
		// Performing the retries performed calculation inside a go-routine to evaluate at defer-time
		client.CountRetries(float64(maxRetries - retries))
	}()
	for retries > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err = store(ctx, client)

		if err == nil {
			return nil
		}

		if _, ok := err.(recoverableError); !ok {
			return err
		}

		retries--

		if retries > 0 {
			time.Sleep(backoff)
			backoff = clampBackoff(2 * backoff)
		}
	}

	return err
}

// sendSamples to the remote storage with backoff for recoverable errors.
func SendSamplesWithBackoff(ctx context.Context, client *Client, samples []prompb.TimeSeries, buf *[]byte) error {
	req, _, err := buildTimeSeriesWriteRequest(samples, *buf)
	*buf = req
	if err != nil {
		// Failing to build the write request is non-recoverable, since it will
		// only error if marshaling the proto to bytes fails.
		return err
	}

	return SendBytesWithBackoff(ctx, client, req)
}

func buildTimeSeriesWriteRequest(samples []prompb.TimeSeries, buf []byte) ([]byte, int64, error) {
	var highest int64
	for _, ts := range samples {
		// At the moment we only ever append a TimeSeries with a single sample in it.
		if ts.Samples[0].Timestamp > highest {
			highest = ts.Samples[0].Timestamp
		}
	}
	req := &prompb.WriteRequest{
		Timeseries: samples,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return nil, highest, err
	}

	// snappy uses len() to see if it needs to allocate a new slice. Make the
	// buffer as long as possible.
	if buf != nil {
		buf = buf[0:cap(buf)]
	}
	compressed := snappy.Encode(buf, data)
	return compressed, highest, nil
}

type closeIdler interface {
	CloseIdleConnections()
}

// BasicAuth contains basic HTTP authentication credentials.
type BasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password,omitempty"`
}

// TLSConfig configures the options for TLS connections.
type TLSConfig struct {
	// The CA cert to use for the targets.
	CAFile string `yaml:"ca_file,omitempty"`
	// The client cert file for the targets.
	CertFile string `yaml:"cert_file,omitempty"`
	// The client key file for the targets.
	KeyFile string `yaml:"key_file,omitempty"`
	// Used to verify the hostname for the targets.
	ServerName string `yaml:"server_name,omitempty"`
	// Disable target certificate validation.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

// HTTPClientConfig configures an HTTP client.
type HTTPClientConfig struct {
	// The HTTP basic authentication credentials for the targets.
	BasicAuth *BasicAuth `yaml:"basic_auth,omitempty"`
	// TLSConfig to use to connect to the targets.
	TLSConfig TLSConfig `yaml:"tls_config,omitempty"`
}

// ClientConfig configures a Client.
type ClientConfig struct {
	URL              *url.URL
	Timeout          time.Duration
	HTTPClientConfig HTTPClientConfig
	UserAgent        string
	Headers          map[string]string
}

type counterMetricFunc func(float64)

type Client struct {
	remoteName       string
	url              *url.URL
	client           *http.Client
	timeout          time.Duration
	headers          map[string]string
	countRetriesFunc counterMetricFunc
}

// NewClient creates a new Client.
func NewClient(remoteName string, conf *ClientConfig, retriesCounter counterMetricFunc) (*Client, error) {
	httpClient, err := NewClientFromConfig(conf.HTTPClientConfig, "remote_storage", false)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"User-Agent": conf.UserAgent,
	}

	// Copy other headers
	for k, v := range conf.Headers {
		headers[k] = v
	}

	return &Client{
		remoteName:       remoteName,
		url:              conf.URL,
		client:           httpClient,
		timeout:          conf.Timeout,
		headers:          headers,
		countRetriesFunc: retriesCounter,
	}, nil
}

// NewClient returns a http.Client using the specified http.RoundTripper.
func newClient(rt http.RoundTripper) *http.Client {
	return &http.Client{Transport: rt}
}

// NewClientFromConfig returns a new HTTP client configured for the
// given config.HTTPClientConfig. The name is used as go-conntrack metric label.
func NewClientFromConfig(cfg HTTPClientConfig, name string, disableKeepAlives bool) (*http.Client, error) {
	rt, err := newRoundTripperFromConfig(cfg, name, disableKeepAlives)
	if err != nil {
		return nil, err
	}
	return newClient(rt), nil
}

// newRoundTripperFromConfig returns a new HTTP RoundTripper configured for the
// given config.HTTPClientConfig. The name is used as go-conntrack metric label.
func newRoundTripperFromConfig(cfg HTTPClientConfig, name string, disableKeepAlives bool) (http.RoundTripper, error) {
	newRT := func(tlsConfig *tls.Config) (http.RoundTripper, error) {
		// The only timeout we care about is the configured scrape timeout.
		// It is applied on request. So we leave out any timings here.
		var rt http.RoundTripper = &http.Transport{
			MaxIdleConns:        20000,
			MaxIdleConnsPerHost: 1000, // see https://github.com/golang/go/issues/13801
			DisableKeepAlives:   disableKeepAlives,
			TLSClientConfig:     tlsConfig,
			DisableCompression:  true,
			// 5 minutes is typically above the maximum sane scrape interval. So we can
			// use keepalive for all configurations.
			IdleConnTimeout:       5 * time.Minute,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			Proxy:                 http.ProxyFromEnvironment,
			DialContext: conntrack.NewDialContextFunc(
				conntrack.DialWithTracing(),
				conntrack.DialWithName(name),
			),
		}

		if cfg.BasicAuth != nil {
			rt = newBasicAuthRoundTripper(cfg.BasicAuth.Username, cfg.BasicAuth.Password, rt)
		}
		// Return a new configured RoundTripper.
		return rt, nil
	}

	tlsConfig, err := NewTLSConfig(&cfg.TLSConfig)
	if err != nil {
		return nil, err
	}

	if len(cfg.TLSConfig.CAFile) == 0 {
		// No need for a RoundTripper that reloads the CA file automatically.
		return newRT(tlsConfig)
	}

	return newTLSRoundTripper(tlsConfig, cfg.TLSConfig.CAFile, newRT)
}

// NewTLSConfig creates a new tls.Config from the given TLSConfig.
func NewTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	// TODO(mem): figure out if there's a reasonable way to deal
	// with this.
	//
	// #nosec
	tlsConfig := &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify}

	// If a CA cert is provided then let's read it in so we can validate the
	// scrape target's certificate properly.
	if len(cfg.CAFile) > 0 {
		b, err := readCAFile(cfg.CAFile)
		if err != nil {
			return nil, err
		}
		if !updateRootCA(tlsConfig, b) {
			return nil, fmt.Errorf("unable to use specified CA cert %s", cfg.CAFile)
		}
	}

	if len(cfg.ServerName) > 0 {
		tlsConfig.ServerName = cfg.ServerName
	}
	// If a client cert & key is provided then configure TLS config accordingly.
	switch {
	case len(cfg.CertFile) > 0 && len(cfg.KeyFile) == 0:
		return nil, fmt.Errorf("client cert file %q specified without client key file", cfg.CertFile)
	case len(cfg.KeyFile) > 0 && len(cfg.CertFile) == 0:
		return nil, fmt.Errorf("client key file %q specified without client cert file", cfg.KeyFile)
	case len(cfg.CertFile) > 0 && len(cfg.KeyFile) > 0:
		// Verify that client cert and key are valid.
		if _, err := cfg.getClientCertificate(nil); err != nil {
			return nil, err
		}
		tlsConfig.GetClientCertificate = cfg.getClientCertificate
	}

	return tlsConfig, nil
}

// getClientCertificate reads the pair of client cert and key from disk and returns a tls.Certificate.
func (c *TLSConfig) getClientCertificate(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("unable to use specified client cert (%s) & key (%s): %s", c.CertFile, c.KeyFile, err)
	}
	return &cert, nil
}

type basicAuthRoundTripper struct {
	username string
	password string
	rt       http.RoundTripper
}

// newBasicAuthRoundTripper will apply a BASIC auth authorization header to a request unless it has
// already been set.
func newBasicAuthRoundTripper(username string, password string, rt http.RoundTripper) http.RoundTripper {
	return &basicAuthRoundTripper{username, password, rt}
}

func (rt *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.rt.RoundTrip(req)
	}
	req = cloneRequest(req)
	req.SetBasicAuth(rt.username, strings.TrimSpace(rt.password))
	return rt.rt.RoundTrip(req)
}

// CountRetries registers the number of retries the client did internally, in a generic metric. The main purpose is to
// wrap in a simple way the underlying Prometheus metric.
func (c *Client) CountRetries(retries float64) {
	c.countRetriesFunc(retries)
}

// StoreBytes sends a batch of samples to the HTTP endpoint, the request is the proto marshalled
// and encoded bytes from codec.go.
func (c *Client) StoreBytes(ctx context.Context, req []byte) error {
	return c.StoreStream(ctx, bytes.NewReader(req))
}

// StoreStream sends a batch of samples to the HTTP endpoint, the request is the proto marshalled
// and encoded bytes from codec.go.
func (c *Client) StoreStream(ctx context.Context, req io.Reader) error {
	// Setup the new request...
	httpReq, err := http.NewRequest("POST", c.url.String(), req)
	if err != nil {
		// Errors from NewRequest are from unparsable URLs, so are not
		// recoverable.
		return err
	}
	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")

	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	// ... and add a context with timeout as late as possible to give it as
	// much chance to finish as possible.
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	httpResp, err := c.client.Do(httpReq.WithContext(ctx))
	if err != nil {
		// Errors from client.Do are from (for example) network errors, so are
		// recoverable.
		return NewRecoverableError(err)
	}
	defer func() {
		// we are draining the body, so it's OK to ignore the
		// return value from io.Copy.
		_, _ = io.Copy(io.Discard, httpResp.Body)

		// If there are close errors we cannot do anything about
		// them.
		_ = httpResp.Body.Close()
	}()

	if httpResp.StatusCode/100 != 2 {
		scanner := bufio.NewScanner(io.LimitReader(httpResp.Body, maxErrMsgLen))
		line := ""
		if scanner.Scan() {
			line = scanner.Text()
		}
		err = &HttpError{
			Status:     httpResp.Status,
			StatusCode: httpResp.StatusCode,
			Err:        errors.Errorf("server returned HTTP status %s: %s", httpResp.Status, line),
		}
	}
	if httpResp.StatusCode/100 == 5 {
		return NewRecoverableError(err)
	}
	return err
}

// tlsRoundTripper is a RoundTripper that updates automatically its TLS
// configuration whenever the content of the CA file changes.
type tlsRoundTripper struct {
	caFile string
	// newRT returns a new RoundTripper.
	newRT func(*tls.Config) (http.RoundTripper, error)

	mtx        sync.RWMutex
	rt         http.RoundTripper
	hashCAFile []byte
	tlsConfig  *tls.Config
}

func newTLSRoundTripper(
	cfg *tls.Config,
	caFile string,
	newRT func(*tls.Config) (http.RoundTripper, error),
) (http.RoundTripper, error) {
	t := &tlsRoundTripper{
		caFile:    caFile,
		newRT:     newRT,
		tlsConfig: cfg,
	}

	rt, err := t.newRT(t.tlsConfig)
	if err != nil {
		return nil, err
	}
	t.rt = rt

	_, t.hashCAFile, err = t.getCAWithHash()
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (t *tlsRoundTripper) getCAWithHash() ([]byte, []byte, error) {
	b, err := readCAFile(t.caFile)
	if err != nil {
		return nil, nil, err
	}
	h := sha256.Sum256(b)
	return b, h[:], nil
}

// RoundTrip implements the http.RoundTrip interface.
func (t *tlsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	b, h, err := t.getCAWithHash()
	if err != nil {
		return nil, err
	}

	t.mtx.RLock()
	equal := bytes.Equal(h, t.hashCAFile)
	rt := t.rt
	t.mtx.RUnlock()
	if equal {
		// The CA cert hasn't changed, use the existing RoundTripper.
		return rt.RoundTrip(req)
	}

	// Create a new RoundTripper.
	tlsConfig := t.tlsConfig.Clone()
	if !updateRootCA(tlsConfig, b) {
		return nil, fmt.Errorf("unable to use specified CA cert %s", t.caFile)
	}
	rt, err = t.newRT(tlsConfig)
	if err != nil {
		return nil, err
	}
	t.CloseIdleConnections()

	t.mtx.Lock()
	t.rt = rt
	t.hashCAFile = h
	t.mtx.Unlock()

	return rt.RoundTrip(req)
}

func (t *tlsRoundTripper) CloseIdleConnections() {
	t.mtx.RLock()
	defer t.mtx.RUnlock()
	if ci, ok := t.rt.(closeIdler); ok {
		ci.CloseIdleConnections()
	}
}

// readCAFile reads the CA cert file from disk.
func readCAFile(f string) ([]byte, error) {
	// gosec complains that 'data' will contain a file.
	//
	// #nosec
	data, err := os.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("unable to load specified CA cert %s: %s", f, err)
	}
	return data, nil
}

// updateRootCA parses the given byte slice as a series of PEM encoded certificates and updates tls.Config.RootCAs.
func updateRootCA(cfg *tls.Config, b []byte) bool {
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(b) {
		return false
	}
	cfg.RootCAs = caCertPool
	return true
}

// cloneRequest returns a clone of the provided *http.Request.
// The clone is a shallow copy of the struct and its Header map.
func cloneRequest(r *http.Request) *http.Request {
	// Shallow copy of the struct.
	r2 := new(http.Request)
	*r2 = *r
	// Deep copy of the Header.
	r2.Header = make(http.Header)
	for k, s := range r.Header {
		r2.Header[k] = s
	}
	return r2
}
