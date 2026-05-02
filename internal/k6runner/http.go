package k6runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

type HttpRunner struct {
	url    string
	logger *zerolog.Logger
	// backoff sets the minimum amount of time to wait before retrying a request. nth attempt waits n times this value,
	// plus some jitter.
	backoff time.Duration
	// graceTime tells the HttpRunner how much time to add to the script timeout to form the request timeout.
	graceTime time.Duration
	// metrics stores metrics for the remote k6 runner.
	metrics *HTTPMetrics
	// versionPollInterval specifies how often Versions() should poll the remote k6 runner for versions and feed that to
	// the channel.
	versionPollInterval time.Duration
}

const (
	defaultBackoff   = 10 * time.Second
	defaultGraceTime = 20 * time.Second
)

// Per-tier agent-side grace times. These are added on top of the
// script timeout to bound the agent's wait for the runner to respond.
// They match the per-stage P99 SLO sums in the runner-service spec
// (small: 1s dispatch + 500ms setup/teardown + RTT ≈ 1.7s → 3s; browser:
// 1s + 5s + RTT ≈ 6.2s → 8s).
const (
	GraceTimeSmall   = 3 * time.Second
	GraceTimeBrowser = 8 * time.Second
)

// GraceFor returns the agent-side request budget grace for the given
// check type. Use this when talking to the new k6-runner service. The
// existing in-house runner integration continues to use the legacy
// [defaultGraceTime] until the migration is complete.
func GraceFor(checkType string) time.Duration {
	switch checkType {
	case "browser":
		return GraceTimeBrowser
	case "scripted", "multihttp":
		return GraceTimeSmall
	default:
		return defaultGraceTime
	}
}

type requestError struct {
	Err     string `json:"error"`
	Message string `json:"msg"`
}

var _ error = requestError{}

func (r requestError) Error() string {
	return fmt.Sprintf("%s: %s", r.Err, r.Message)
}

// HTTPRunRequest
type HTTPRunRequest struct {
	Script      `json:",inline"`
	SecretStore SecretStore `json:",inline"`
	NotAfter    time.Time   `json:"notAfter"`
	// CallerHint records the source of the request ("scheduled" or "adhoc") for telemetry labelling on the runner
	// side. The runner makes no behavioural decision based on this value.
	CallerHint string `json:"callerHint,omitempty"`
}

// Caller hint values for [HTTPRunRequest.CallerHint].
const (
	CallerHintScheduled = "scheduled"
	CallerHintAdhoc     = "adhoc"
)

// DrainHeader is the response header set by the dispatcher when a request is being returned because the dispatcher
// is draining. Clients seeing this header should reschedule immediately with no backoff and without counting the
// outcome as a failure.
const DrainHeader = "X-K6-Runner-Drain"

type RunResponse struct {
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
	Metrics   []byte `json:"metrics"`
	Logs      []byte `json:"logs"`
}

type VersionsResponse struct {
	List []struct {
		Version string `json:"version"`
		Path    string `json:"path"` // Unused.
	} `json:"versions"`
}

func (v VersionsResponse) Versions() []string {
	versions := make([]string, 0, len(v.List))
	for _, v := range v.List {
		versions = append(versions, v.Version)
	}
	return versions
}

func (r HttpRunner) WithLogger(logger *zerolog.Logger) Runner {
	r.logger = logger

	return r
}

var ErrUnexpectedStatus = errors.New("unexpected status code")

func (r HttpRunner) Run(ctx context.Context, script Script, secretStore SecretStore) (*RunResponse, error) {
	if r.backoff == 0 {
		panic("zero backoff, runner is misconfigured, refusing to DoS")
	}

	// FIXME: Make this an error after we fully roll out k6 versioning.
	if script.K6ChannelManifest == "" || script.K6ChannelManifest == "*" {
		r.logger.Debug().Msg("Check does not define a channel, latest k6 version will be used and it may change without warning")
	}

	if deadline, hasDeadline := ctx.Deadline(); !hasDeadline {
		defaultAllRetriesTimeout := time.Duration(script.Settings.Timeout) * time.Millisecond * 2
		r.logger.Error().
			Dur("allRetriesTimeout", defaultAllRetriesTimeout).
			Msg("k6 runner does not have a deadline for all retries. This is a bug. Defaulting to twice the timeout to avoid retrying forever")

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultAllRetriesTimeout)
		defer cancel()
	} else if tud := time.Until(deadline); tud < time.Duration(script.Settings.Timeout)*time.Millisecond*2 {
		r.logger.Debug().
			Str("timeUntilNext", tud.String()).
			Str("timeout", (time.Duration(script.Settings.Timeout) * time.Millisecond).String()).
			Msg("time until next execution is too close to script timeout, there might not be room for retries")
	}

	// Retry logic is purely context (time) based, but we keep track of the number of attempts for reporting telemetry.
	wait := r.backoff
	var attempts float64
	var response *RunResponse
	for {
		start := time.Now()

		attempts += 1
		var err error
		response, err = r.request(ctx, script, secretStore)
		if err == nil {
			r.logger.Debug().Bytes("metrics", response.Metrics).Bytes("logs", response.Logs).Msg("script result")
			r.metrics.Requests.With(map[string]string{metricLabelSuccess: "1", metricLabelRetriable: ""}).Inc()
			r.metrics.RequestsPerRun.WithLabelValues("1").Observe(attempts)
			return response, nil
		}

		if !errors.Is(err, errRetryable) {
			// TODO: Log the returned error in the Processor instead.
			r.logger.Error().Err(err).Msg("non-retryable error running k6")
			r.metrics.Requests.With(map[string]string{metricLabelSuccess: "0", metricLabelRetriable: "0"}).Inc()
			r.metrics.RequestsPerRun.WithLabelValues("0").Observe(attempts)
			return nil, err
		}

		// A drain response is not a failure: the dispatcher is being deliberately torn down and the agent must retry
		// immediately. We do not increment the retriable-failure counter for drain so the metric stays a clean signal
		// of real transient errors. We also skip the wait timer entirely, since the spec requires immediate retry.
		if errors.Is(err, errDrain) {
			r.metrics.DrainRetries.Inc()
			r.logger.Info().Msg("dispatcher draining; retrying immediately")
			if ctx.Err() != nil {
				r.metrics.RequestsPerRun.WithLabelValues("0").Observe(attempts)
				return nil, fmt.Errorf("cannot retry further: %w", errors.Join(err, ctx.Err()))
			}
			continue
		}

		r.metrics.Requests.With(map[string]string{metricLabelSuccess: "0", metricLabelRetriable: "1"}).Inc()

		// Wait, but subtract the amount of time we've already waited as part of the request timeout.
		// We do this because these requests have huge timeouts, and by the nature of the system running these requests,
		// we expect the most common error to be a timeout, so we avoid waiting even more on top of an already large
		// value.
		waitRemaining := max(0, wait-time.Since(start))
		r.logger.Warn().Err(err).Dur("after", waitRemaining).Msg("retrying retryable error")

		waitTimer := time.NewTimer(waitRemaining)
		select {
		case <-ctx.Done():
			waitTimer.Stop()
			// TODO: Log the returned error in the Processor instead.
			r.logger.Error().Err(err).Object("checkInfo", &script.CheckInfo).Msg("retries exhausted")
			r.metrics.RequestsPerRun.WithLabelValues("0").Observe(attempts)
			return nil, fmt.Errorf("cannot retry further: %w", errors.Join(err, ctx.Err()))
		case <-waitTimer.C:
		}

		// Backoff linearly, adding some jitter.
		wait += r.backoff + time.Duration(rand.Int64N(int64(r.backoff)))
	}
}

// errRetryable indicates that an error is retryable. It is typically joined with another error.
var errRetryable = errors.New("retryable")

// errDrain indicates that the runner service is being drained and the
// request should be retried immediately, with no backoff and without
// counting the outcome as a check failure. The retry loop in [HttpRunner.Run]
// recognises this sentinel and skips its wait timer and failure-count
// metric.
var errDrain = errors.New("dispatcher draining")

func (r HttpRunner) request(ctx context.Context, script Script, secretStore SecretStore) (*RunResponse, error) {
	checkTimeout := time.Duration(script.Settings.Timeout) * time.Millisecond
	if checkTimeout == 0 {
		return nil, ErrNoTimeout
	}

	// requestTimeout should be noticeably larger than [Script.Settings.Timeout], to account for added latencies in the
	// system such as network, i/o, seralization, queue wait time, etc. that take place after and before the script is
	// ran.
	//  t0                 t1                                      t2               t3
	//  |--- Queue wait ---|-------------- k6 run -----------------|--- Response ---|
	//  checkTimeout = t2 - t1
	//  requestTimeout = t3 - t0
	requestTimeout := checkTimeout + r.graceTime
	notAfter := time.Now().Add(requestTimeout)

	ctx, cancel := context.WithDeadline(ctx, notAfter)
	defer cancel()

	// Decorate the script request with the NotAfter hint.
	// NotAfter hints runners when we're about to drop this request. Runners will refuse to start to run a script if
	// this time is in the past, as it is guaranteed that we, the client, have already given up on the request.
	// This allows runners to not waste time running scripts which will not complete before the client gives up on the
	// request.
	runRequest := HTTPRunRequest{
		Script:      script,
		SecretStore: secretStore,
		NotAfter:    notAfter,
	}

	reqBody, err := json.Marshal(runRequest)
	if err != nil {
		return nil, fmt.Errorf("encoding script: %w", err)
	}

	runURL, err := url.JoinPath(r.url, "/run")
	if err != nil {
		return nil, fmt.Errorf("building URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, runURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req.Header.Add("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.logger.Error().Err(err).Msg("sending request")

		// Any error making a request is retryable.
		return nil, errors.Join(errRetryable, fmt.Errorf("making request: %w", err))
	}

	defer resp.Body.Close()

	// The dispatcher of the new k6-runner service tags drain responses with [DrainHeader]. A drain response means
	// "reschedule this request immediately, with no backoff, and don't count it as a failure" — see the spec under
	// Failure & retry semantics → Dispatcher drain / shutdown.
	if resp.Header.Get(DrainHeader) != "" {
		return nil, errors.Join(errRetryable, errDrain)
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusRequestTimeout, http.StatusUnprocessableEntity, http.StatusInternalServerError:
	// These are status code that we assume come with a machine-readable response. The response may contain an error, which is
	// handled later.
	// See: https://github.com/grafana/sm-k6-runner/blob/main/internal/mq/proxy.go#L215

	case http.StatusBadRequest:
		// These are status codes that do not come with a machine readable response, and are not retryable.
		//
		// There might be an argument to be made to retry 500s, as they can be produced by panic recovering mechanisms which
		// _can_ be seen as a transient error. However, it is also possible for a 500 to be returned by a script that failed
		// and also needed a lot of time to complete. For this reason, we choose to not retry 500 for the time being.
		return nil, fmt.Errorf("%w %d", ErrUnexpectedStatus, resp.StatusCode)

	default:
		// Statuses not returned by the proxy directly are assumed to be infrastructure (e.g. ingress, k8s) related and
		// thus marked as retriable.
		// Runners may also return http.StatusServiceUnavailable if the browser session manager cannot be reached. We want
		// to retry those errors, so we let the "default" block catch them.
		return nil, errors.Join(errRetryable, fmt.Errorf("%w %d", ErrUnexpectedStatus, resp.StatusCode))
	}

	var response RunResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		r.logger.Error().Err(err).Msg("decoding script result")
		return nil, fmt.Errorf("decoding script result: %w", err)
	}

	return &response, nil
}

func (r HttpRunner) Versions(ctx context.Context) <-chan []string {
	if r.versionPollInterval == 0 {
		panic("zero versionPollInterval, runner is misconfigured")
	}

	ch := make(chan []string)

	go func() {
		ticker := time.NewTicker(r.versionPollInterval)
		defer ticker.Stop()

		r.logger.Debug().Str("url", r.url).Msg("Starting to poll for versions") // TODO: This is not the actual URL.

		for {
			select {
			case <-ctx.Done():
				r.logger.Err(ctx.Err()).Msg("polling for versions stopped")
				close(ch)
				return
			case <-ticker.C:
				err := func() error { // Wrap in function so we can use conventional defer and if-err-nil-return
					rCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
					defer cancel()

					versionsURL, err := url.JoinPath(r.url, "/versions")
					if err != nil {
						return fmt.Errorf("building URL: %w", err)
					}

					req, err := http.NewRequestWithContext(rCtx, http.MethodGet, versionsURL, nil)
					if err != nil {
						return fmt.Errorf("building request: %w", err)
					}

					req.Header.Add("accept", "application/json")

					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						return fmt.Errorf("making request: %w", err)
					}

					defer resp.Body.Close()

					if resp.StatusCode != 200 {
						return fmt.Errorf("unexpected status %d", resp.StatusCode)
					}

					response := VersionsResponse{}
					err = json.NewDecoder(resp.Body).Decode(&response)
					if err != nil {
						return fmt.Errorf("decoding response: %w", err)
					}

					versions := response.Versions()

					r.logger.Debug().Strs("versions", versions).Msg("Polled remote runner for versions")

					select {
					case <-ctx.Done():
						return nil // This error will be caught and reported in the outer loop.
					case <-time.After(3 * time.Second):
						r.logger.Warn().Msg("Version report went unconsumed, discarding report and continuing")
						return nil
					case ch <- versions:
						return nil
					}
				}()
				if err != nil {
					r.logger.Error().Err(err).Msg("requesting versions")
					continue
				}
			}
		}
	}()

	return ch
}

type HTTPMetrics struct {
	Requests       *prometheus.CounterVec
	RequestsPerRun *prometheus.HistogramVec
	// DrainRetries counts requests that were retried because the runner
	// service signalled a drain. These are not counted as retriable
	// failures: a drain is operationally normal, not a transient error.
	DrainRetries prometheus.Counter
}

const (
	metricLabelSuccess   = "success"
	metricLabelRetriable = "retriable"
)

func NewHTTPMetrics(registerer prometheus.Registerer) *HTTPMetrics {
	m := &HTTPMetrics{}
	m.Requests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sm_agent",
			Subsystem: "k6runner",
			Name:      "requests_total",
			Help: "Total number of requests made to remote k6 runners, which may be more than one per run. " +
				"The 'success' label is 1 if this single request succeeded, 0 otherwise. " +
				"The 'retriable' label is 1 if the request failed with a retriable error, 0 otherwise. " +
				"Successful requests do not have the 'retriable' label.",
		},
		[]string{metricLabelSuccess, metricLabelRetriable},
	)
	registerer.MustRegister(m.Requests)

	m.RequestsPerRun = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "sm_agent",
			Subsystem: "k6runner",
			Name:      "requests_per_run",
			Help: "Number of requests attempted per run operation. " +
				"The 'success' label is 1 if one ultimately succeeded potentially including retries, 0 otherwise.",
			// Generally we expect request to be retries a handful of times, so we create high resolution buckets up to
			// 5. Form 5 onwards something off is going on and we do not care that much about resolution.
			Buckets: []float64{1, 2, 3, 4, 5, 10, 20, 50},
		},
		[]string{metricLabelSuccess},
	)
	registerer.MustRegister(m.RequestsPerRun)

	m.DrainRetries = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "sm_agent",
		Subsystem: "k6runner",
		Name:      "request_drain_retries_total",
		Help: "Number of requests that were retried because the runner service signalled a dispatcher drain. " +
			"Drain retries are not counted as failures.",
	})
	registerer.MustRegister(m.DrainRetries)

	return m
}

func trimRunSuffix(rawUrl string) string {
	u, err := url.Parse(rawUrl)
	if err != nil {
		// This URL is not parseable, do not attempt to replace anything. It will error somewhere else down the line.
		return rawUrl
	}

	u.Path = strings.TrimSuffix(u.Path, "/run/")
	u.Path = strings.TrimSuffix(u.Path, "/run")

	// This may leave the URL with an empty path (no sole "/"). This is okay as per
	// https://www.rfc-editor.org/rfc/rfc3986#section-3.3

	return u.String()
}
