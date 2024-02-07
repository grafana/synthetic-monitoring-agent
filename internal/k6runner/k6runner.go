package k6runner

import (
	"bytes"
	"context"
	"encoding/json"
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

type Runner interface {
	WithLogger(logger *zerolog.Logger) Runner
	Run(ctx context.Context, script []byte) (*RunResponse, error)
}

func New(uri string) Runner {
	var r Runner
	logger := zerolog.Nop()

	if strings.HasPrefix(uri, "http") {
		r = &HttpRunner{
			url:    uri,
			logger: &logger,
		}
	} else {
		r = &LocalRunner{
			k6path: uri,
			logger: &logger,
			fs:     afero.NewOsFs(),
		}
	}

	return r
}

type Script struct {
	runner Runner
	script []byte
}

func NewScript(script []byte, k6runner Runner) (*Script, error) {
	r := Script{
		runner: k6runner,
		script: script,
	}

	return &r, nil
}

func (r Script) Run(ctx context.Context, registry *prometheus.Registry, logger logger.Logger, internalLogger zerolog.Logger) (bool, error) {
	k6runner := r.runner.WithLogger(&internalLogger)

	result, err := k6runner.Run(ctx, r.script)
	if err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("k6 script exited with error code")
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

	if err := k6LogsToLogger(result.Logs, logger); err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("cannot load logs to logger")
		return false, err
	}

	return !resultCollector.failure, nil
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
	promDecoder := expfmt.NewDecoder(bytes.NewBuffer(metrics), expfmt.FmtText)
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

type Settings struct {
	Timeout int64 `json:"timeout"`
}

type RunRequest struct {
	Script   []byte   `json:"script"`
	Settings Settings `json:"settings"`
}

type RunResponse struct {
	Metrics []byte `json:"metrics"`
	Logs    []byte `json:"logs"`
}

func (r HttpRunner) WithLogger(logger *zerolog.Logger) Runner {
	return HttpRunner{
		url:    r.url,
		logger: logger,
	}
}

func (r HttpRunner) Run(ctx context.Context, script []byte) (*RunResponse, error) {
	req, err := json.Marshal(&RunRequest{
		Script: script,
		Settings: Settings{
			Timeout: getTimeout(ctx).Milliseconds(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("running script: %w", err)
	}

	resp, err := http.Post(r.url, "application/json", bytes.NewBuffer(req))
	if err != nil {
		r.logger.Error().Err(err).Msg("sending request")
		return nil, fmt.Errorf("running script: %w", err)
	}

	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var result requestError

		err := dec.Decode(&result)
		if err != nil {
			r.logger.Error().Err(err).Msg("decoding request response")
			return nil, fmt.Errorf("running script: %w", err)
		}

		r.logger.Error().Err(result).Msg("request response")
		return nil, fmt.Errorf("running script: %w", result)
	}

	var result RunResponse

	err = dec.Decode(&result)
	if err != nil {
		r.logger.Error().Err(err).Msg("decoding script result")
		return nil, fmt.Errorf("decoding script result: %w", err)
	}

	r.logger.Debug().Bytes("metrics", result.Metrics).Bytes("logs", result.Logs).Msg("script result")

	return &result, nil
}

type LocalRunner struct {
	k6path string
	logger *zerolog.Logger
	fs     afero.Fs
}

func (r LocalRunner) WithLogger(logger *zerolog.Logger) Runner {
	return LocalRunner{
		k6path: r.k6path,
		fs:     r.fs,
		logger: logger,
	}
}

func (r LocalRunner) Run(ctx context.Context, script []byte) (*RunResponse, error) {
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

	if err := afs.WriteFile(scriptFn, script, 0o644); err != nil {
		return nil, fmt.Errorf("cannot write temporary script file: %w", err)
	}

	k6Path, err := exec.LookPath(r.k6path)
	if err != nil {
		return nil, fmt.Errorf("cannot find k6 executable: %w", err)
	}

	timeout := getTimeout(ctx)

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
		"--blacklist-ip", "10.0.0.0/8", // TODO(mem): make this configurable
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

	r.logger.Info().Str("command", cmd.String()).Bytes("script", script).Msg("running k6 script")

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

func getTimeout(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 10 * time.Second
	}

	return time.Until(deadline)
}
