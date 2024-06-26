package k6runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/go-logfmt/logfmt"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

// Script is a k6 script that a runner is able to run, with some added instructions for that runner to act on.
type Script struct {
	Script   []byte   `json:"script"`
	Settings Settings `json:"settings"`
	// TODO: Add Metadata and Features.
}

type Settings struct {
	// Timeout for k6 run, in milliseconds. This value is a configuration value for remote runners, which will instruct
	// them to return an error if the operation takes longer than this time to complete. Clients should expect that
	// requests to remote runners may take longer than this value due to network and other latencies, and thus clients
	// should wait additional time before aborting outgoing requests.
	Timeout int64 `json:"timeout"`
}

type Runner interface {
	WithLogger(logger *zerolog.Logger) Runner
	Run(ctx context.Context, script Script) (*RunResponse, error)
}

type RunnerOpts struct {
	Uri           string
	BlacklistedIP string
}

func New(opts RunnerOpts) Runner {
	var r Runner
	logger := zerolog.Nop()

	if strings.HasPrefix(opts.Uri, "http") {
		r = &HttpRunner{
			url:    opts.Uri,
			logger: &logger,
		}
	} else {
		r = LocalRunner{
			k6path:        opts.Uri,
			logger:        &logger,
			blacklistedIP: opts.BlacklistedIP,
			fs:            afero.NewOsFs(),
		}
	}

	return r
}

// Processor runs a script with a runner and parses the k6 output.
type Processor struct {
	runner Runner
	script Script
}

func NewProcessor(script Script, k6runner Runner) (*Processor, error) {
	r := Processor{
		runner: k6runner,
		script: script,
	}

	return &r, nil
}

var (
	ErrBuggyRunner = errors.New("runner returned buggy response")
	ErrFromRunner  = errors.New("runner reported an error")
)

func (r Processor) Run(ctx context.Context, registry *prometheus.Registry, logger logger.Logger, internalLogger zerolog.Logger) (bool, error) {
	k6runner := r.runner.WithLogger(&internalLogger)

	result, err := k6runner.Run(ctx, r.script)
	if err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("k6 script exited with error code")
		return false, err
	}

	// Send logs before metrics to make sure logs are submitted even if the metrics output is not parsable.
	if err := k6LogsToLogger(result.Logs, logger); err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("cannot load logs to logger")
		return false, err
	}

	var (
		collector       sampleCollector
		resultCollector checkResultCollector
	)

	if err := extractMetricSamples(result.Metrics, internalLogger, collector.process, resultCollector.process); err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("cannot extract metric samples")
		return false, err
	}

	if err := registry.Register(&collector.collector); err != nil {
		internalLogger.Error().
			Err(err).
			Msg("cannot register collector")
		return false, err
	}

	// If only one of Error and ErrorCode are non-empty, the proxy is misbehaving.
	switch {
	case result.Error == "" && result.ErrorCode != "":
		fallthrough
	case result.Error != "" && result.ErrorCode == "":
		return false, fmt.Errorf(
			"%w: only one of error (%q) and errorCode (%q) is non-empty",
			ErrBuggyRunner, result.Error, result.ErrorCode,
		)
	}

	// https://github.com/grafana/sm-k6-runner/blob/b811839d444a7e69fd056b0a4e6ccf7e914197f3/internal/mq/runner.go#L51
	switch result.ErrorCode {
	case "":
		// No error, all good.
		return true, nil
	case "timeout", "killed", "user":
		// These are user errors. The probe failed, but we don't return an error.
		return false, nil
	default:
		// We got an "unknown" error, or some other code we do not recognize. Return it so we log it.
		return false, fmt.Errorf("%w: %s: %s", ErrFromRunner, result.ErrorCode, result.Error)
	}
}

type customCollector struct {
	metrics []prometheus.Metric
}

func (c *customCollector) Describe(ch chan<- *prometheus.Desc) {
	// We do not send any descriptions in order to create an unchecked
	// metric. This allows us to have a metric with the same name and
	// different label values.
	//
	// TODO(mem): reevaluate if this is really want we want to do.
}

func (c *customCollector) Collect(ch chan<- prometheus.Metric) {
	for _, m := range c.metrics {
		ch <- m
	}
}

type sampleProcessorFunc func(*dto.MetricFamily, *model.Sample) error

type sampleCollector struct {
	collector customCollector
}

func (sc *sampleCollector) process(mf *dto.MetricFamily, sample *model.Sample) error {
	// TODO(mem): This is really crappy. We have a
	// set of metric families, and we are
	// converting back to samples so that we can
	// add that to the registry. We need to rework
	// the logic in the prober so that it can
	// return a set of metric families. The probes
	// that don't have this, can create a registry
	// locally and get the metric families from
	// that.
	name, found := sample.Metric[model.MetricNameLabel]
	if !found {
		return fmt.Errorf("missing metric name")
	}

	desc := prometheus.NewDesc(string(name), mf.GetHelp(), nil, metricToLabels(sample.Metric))
	// TODO(mem): maybe change this to untyped?
	m, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, float64(sample.Value))
	if err != nil {
		return fmt.Errorf("creating prometheus metric: %w", err)
	}

	sc.collector.metrics = append(sc.collector.metrics, m)

	return nil
}

type checkResultCollector struct {
	failure bool
}

func (rc *checkResultCollector) process(mf *dto.MetricFamily, sample *model.Sample) error {
	if sample.Metric[model.MetricNameLabel] != "probe_checks_total" {
		return nil
	}

	if sample.Metric["result"] != "fail" {
		return nil
	}

	if sample.Value != 0 {
		rc.failure = true
	}

	return nil
}

func extractMetricSamples(metrics []byte, logger zerolog.Logger, processors ...sampleProcessorFunc) error {
	promDecoder := expfmt.NewDecoder(bytes.NewBuffer(metrics), expfmt.NewFormat(expfmt.TypeTextPlain))
	decoderOpts := expfmt.DecodeOptions{Timestamp: model.Now()}
	for {
		var mf dto.MetricFamily
		switch err := promDecoder.Decode(&mf); err {
		case nil:
			// Got one metric family
			samples, err := expfmt.ExtractSamples(&decoderOpts, &mf)
			if err != nil {
				logger.Error().Err(err).Msg("extracting samples")
				return fmt.Errorf("extracting samples: %w", err)
			}

			for _, sample := range samples {
				for _, p := range processors {
					err := p(&mf, sample)
					if err != nil {
						logger.Error().Err(err).Msg("processing samples")
						return err
					}
				}
			}

		case io.EOF:
			// nothing was returned, we are done
			return nil

		default:
			logger.Error().Err(err).Msg("decoding prometheus metrics")
			return fmt.Errorf("decoding prometheus metrics: %w", err)
		}
	}
}

func metricToLabels(metrics model.Metric) prometheus.Labels {
	// Ugh.
	labels := make(prometheus.Labels)
	for name, value := range metrics {
		name := string(name)
		if name == model.MetricNameLabel {
			continue
		}
		labels[name] = string(value)
	}
	return labels
}

func k6LogsToLogger(logs []byte, logger logger.Logger) error {
	// This seems a little silly, we should be able to take the out of k6
	// and pass it directly to Loki. The problem with that is that the only
	// thing probers have access to is the logger.Logger.
	//
	// We could pass another object to the prober, that would take Loki log
	// entries, and the publisher could decorate that with the necessary
	// labels.
	dec := logfmt.NewDecoder(bytes.NewBuffer(logs))

NEXT_RECORD:
	for dec.ScanRecord() {
		var line []interface{}
		var source, level string
		for dec.ScanKeyval() {
			key := string(dec.Key())
			value := string(dec.Value())
			switch key {
			case "source":
				source = value
			case "level":
				level = value
			}
			line = append(line, key, value)
		}
		if level == "debug" && source == "" { // if there's no source, it's probably coming from k6
			continue NEXT_RECORD
		}
		_ = logger.Log(line...)
	}

	return nil
}

type HttpRunner struct {
	url    string
	logger *zerolog.Logger
}

type requestError struct {
	Err     string `json:"error"`
	Message string `json:"msg"`
}

var _ error = requestError{}

func (r requestError) Error() string {
	return fmt.Sprintf("%s: %s", r.Err, r.Message)
}

type RunResponse struct {
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
	Metrics   []byte `json:"metrics"`
	Logs      []byte `json:"logs"`
}

func (r HttpRunner) WithLogger(logger *zerolog.Logger) Runner {
	return HttpRunner{
		url:    r.url,
		logger: logger,
	}
}

var ErrUnexpectedStatus = errors.New("unexpected status code")

func (r HttpRunner) Run(ctx context.Context, script Script) (*RunResponse, error) {
	k6Timeout := time.Duration(script.Settings.Timeout) * time.Millisecond

	reqBody, err := json.Marshal(script)
	if err != nil {
		return nil, fmt.Errorf("encoding script: %w", err)
	}

	// The context above carries the check timeout, which will be eventually passed to k6 by the runner at the other end
	// of this request. To account for network overhead, we create a different context with an extra second of timeout,
	// which adds some grace time to account for the network/system latency of the http request.
	reqCtx, cancel := context.WithTimeout(context.Background(), k6Timeout+time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, r.url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req.Header.Add("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.logger.Error().Err(err).Msg("sending request")
		return nil, fmt.Errorf("running script: %w", err)
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusRequestTimeout, http.StatusUnprocessableEntity, http.StatusInternalServerError:
	// These are status code that come with a machine-readable response. The response may contain an error, which is
	// handled later.
	// See: https://github.com/grafana/sm-k6-runner/blob/main/internal/mq/proxy.go#L215
	default:
		return nil, fmt.Errorf("%w %d", ErrUnexpectedStatus, resp.StatusCode)
	}

	var result RunResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		r.logger.Error().Err(err).Msg("decoding script result")
		return nil, fmt.Errorf("decoding script result: %w", err)
	}

	r.logger.Debug().Bytes("metrics", result.Metrics).Bytes("logs", result.Logs).Msg("script result")

	return &result, nil
}

type LocalRunner struct {
	k6path        string
	logger        *zerolog.Logger
	fs            afero.Fs
	blacklistedIP string
}

func (r LocalRunner) WithLogger(logger *zerolog.Logger) Runner {
	r.logger = logger
	return r
}

func (r LocalRunner) Run(ctx context.Context, script Script) (*RunResponse, error) {
	afs := afero.Afero{Fs: r.fs}

	workdir, err := afs.TempDir("", "k6-runner")
	if err != nil {
		return nil, fmt.Errorf("cannot create temporary directory: %w", err)
	}

	defer func() {
		if err := r.fs.RemoveAll(workdir); err != nil {
			r.logger.Error().Err(err).Str("severity", "critical").Msg("cannot remove temporary directory")
		}
	}()

	metricsFn, err := mktemp(r.fs, workdir, "*.json")
	if err != nil {
		return nil, fmt.Errorf("cannot obtain temporary metrics filename: %w", err)
	}

	logsFn, err := mktemp(r.fs, workdir, "*.log")
	if err != nil {
		return nil, fmt.Errorf("cannot obtain temporary logs filename: %w", err)
	}

	scriptFn, err := mktemp(r.fs, workdir, "*.js")
	if err != nil {
		return nil, fmt.Errorf("cannot obtain temporary script filename: %w", err)
	}

	if err := afs.WriteFile(scriptFn, script.Script, 0o644); err != nil {
		return nil, fmt.Errorf("cannot write temporary script file: %w", err)
	}

	k6Path, err := exec.LookPath(r.k6path)
	if err != nil {
		return nil, fmt.Errorf("cannot find k6 executable: %w", err)
	}

	timeout := time.Duration(script.Settings.Timeout) * time.Millisecond

	// #nosec G204 -- the variables are not user-controlled
	cmd := exec.CommandContext(
		ctx,
		k6Path,
		"run",
		"--out", "sm="+metricsFn,
		"--log-format", "logfmt",
		"--log-output", "file="+logsFn,
		"--vus", "1",
		"--iterations", "1",
		"--duration", timeout.String(),
		"--max-redirects", "10",
		"--batch", "10",
		"--batch-per-host", "4",
		"--no-connection-reuse",
		"--blacklist-ip", r.blacklistedIP,
		"--block-hostnames", "*.cluster.local", // TODO(mem): make this configurable
		"--summary-time-unit", "s",
		// "--discard-response-bodies",                        // TODO(mem): make this configurable
		"--dns", "ttl=30s,select=random,policy=preferIPv4", // TODO(mem): this needs fixing, preferIPv4 is probably not what we want
		"--no-thresholds",
		"--no-usage-report",
		"--no-color",
		"--no-summary",
		"--verbose",
		scriptFn,
	)

	var stdout, stderr bytes.Buffer

	cmd.Dir = workdir
	cmd.Stdin = nil
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()

	r.logger.Info().Str("command", cmd.String()).Bytes("script", script.Script).Msg("running k6 script")

	if err := cmd.Run(); err != nil {
		r.logger.Error().Err(err).Str("stdout", stdout.String()).Str("stderr", stderr.String()).Msg("k6 exited with error")
		return nil, fmt.Errorf("cannot execute k6 script: %w", err)
	}

	duration := time.Since(start)

	r.logger.Debug().Str("stdout", stdout.String()).Str("stderr", stderr.String()).Dur("duration", duration).Msg("ran k6 script")
	r.logger.Info().Dur("duration", duration).Msg("ran k6 script")

	var result RunResponse

	result.Metrics, err = afs.ReadFile(metricsFn)
	if err != nil {
		r.logger.Error().Err(err).Str("filename", metricsFn).Msg("cannot read metrics file")
		return nil, fmt.Errorf("cannot read metrics: %w", err)
	}

	result.Logs, err = afs.ReadFile(logsFn)
	if err != nil {
		r.logger.Error().Err(err).Str("filename", logsFn).Msg("cannot read metrics file")
		return nil, fmt.Errorf("cannot read logs: %w", err)
	}

	r.logger.Debug().Bytes("metrics", result.Metrics).Bytes("logs", result.Logs).Msg("k6 result")

	return &result, nil
}

func mktemp(fs afero.Fs, dir, pattern string) (string, error) {
	f, err := afero.TempFile(fs, dir, pattern)
	if err != nil {
		return "", fmt.Errorf("cannot create temporary file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("cannot close temporary file: %w", err)
	}
	return f.Name(), nil
}
