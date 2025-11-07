package k6runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-logfmt/logfmt"
	smmmodel "github.com/grafana/synthetic-monitoring-agent/internal/model"
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
	// Script is the blob of bytes that is to be run.
	Script []byte `json:"script"`
	// Settings is a common representation of the fields common to all implementation-specific check settings that the
	// runners are interested about.
	Settings Settings `json:"settings"`
	// CheckInfo holds information about the SM check that triggered this script.
	CheckInfo CheckInfo `json:"check"`
	// SecretStore holds the location and token for accessing secrets
}

type SecretStore struct {
	Url   string `json:"url"`
	Token string `json:"token"`
}

// IsConfigured returns true if the SecretStore has both URL and token configured.
func (s SecretStore) IsConfigured() bool {
	return s.Url != "" && s.Token != ""
}

// Settings is a common representation of the fields common to all implementation-specific check settings that the
// runners are interested about.
type Settings struct {
	// Timeout for k6 run, in milliseconds. This value is a configuration value for remote runners, which will instruct
	// them to return an error if the operation takes longer than this time to complete. Clients should expect that
	// requests to remote runners may take longer than this value due to network and other latencies, and thus clients
	// should wait additional time before aborting outgoing requests.
	Timeout int64 `json:"timeout"`
}

// CheckInfo holds information about the SM check that triggered this script.
type CheckInfo struct {
	// Type is the string representation of the check type this script belongs to (browser, scripted, multihttp, etc.)
	Type string `json:"type"`
	// Metadata is a collection of key/value pairs containing information about this check, such as check and tenant ID.
	// It is loosely typed on purpose: Metadata should only be used for informational properties that will make its way
	// into telemetry, and not for making decision on it.
	Metadata map[string]any `json:"metadata"`
}

// CheckInfoFromSM returns a CheckInfo from the information of the given SM check.
func CheckInfoFromSM(smc smmmodel.Check) CheckInfo {
	ci := CheckInfo{
		Metadata: map[string]any{},
	}

	ci.Type = smc.Type().String()
	ci.Metadata["id"] = smc.Id
	ci.Metadata["tenantID"] = smc.TenantId
	ci.Metadata["regionID"] = smc.RegionId
	ci.Metadata["created"] = smc.Created
	ci.Metadata["modified"] = smc.Modified

	return ci
}

// MarshalZerologObject implements zerolog.LogObjectMarshaler so it can be logged in a friendly way.
func (ci *CheckInfo) MarshalZerologObject(e *zerolog.Event) {
	e.Str("type", ci.Type)
	for k, v := range ci.Metadata {
		e.Any(k, v)
	}
}

// ErrNoTimeout is returned by [Runner] implementations if the supplied script has a timeout of zero.
var ErrNoTimeout = errors.New("check has no timeout")

type Runner interface {
	WithLogger(logger *zerolog.Logger) Runner
	Run(ctx context.Context, script Script, secretStore SecretStore) (*RunResponse, error)
}

type RunnerOpts struct {
	Uri           string
	BlacklistedIP string
	Registerer    prometheus.Registerer
}

func New(opts RunnerOpts) Runner {
	var r Runner
	logger := zerolog.Nop()
	var registerer prometheus.Registerer = prometheus.NewRegistry() // NOOP registry.
	if opts.Registerer != nil {
		registerer = opts.Registerer
	}

	if strings.HasPrefix(opts.Uri, "http") {
		r = &HttpRunner{
			url:       opts.Uri,
			logger:    &logger,
			graceTime: defaultGraceTime,
			backoff:   defaultBackoff,
			metrics:   NewHTTPMetrics(registerer),
		}
	} else {
		r = Local{
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

func (r Processor) Run(ctx context.Context, registry *prometheus.Registry, logger logger.Logger, internalLogger zerolog.Logger, secretStore SecretStore) (bool, time.Duration, error) {
	k6runner := r.runner.WithLogger(&internalLogger)

	// TODO: This error message is okay to be Debug for local k6 execution, but should be Error for remote runners.
	result, err := k6runner.Run(ctx, r.script, secretStore)
	if err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("k6 script exited with error code")
		return false, 0, err
	}

	// If only one of Error and ErrorCode are non-empty, the proxy is misbehaving.
	switch {
	case result.Error == "" && result.ErrorCode != "":
		fallthrough
	case result.Error != "" && result.ErrorCode == "":
		return false, 0, fmt.Errorf(
			"%w: only one of error (%q) and errorCode (%q) is non-empty",
			ErrBuggyRunner, result.Error, result.ErrorCode,
		)
	}

	// If the script was not successful, send a log line saying why.
	// Do this in a deferred function to ensure that we send it both after script logs, and regardless of errors sending
	// other logs.
	if result.ErrorCode != "" {
		defer func() {
			err := logger.Log(
				"level", "error",
				"msg", "script did not execute successfully",
				"error", result.Error,
				"errorCode", result.ErrorCode,
			)
			if err != nil {
				internalLogger.Error().
					Err(err).
					Msg("sending diagnostic log")
			}
		}()
	}

	// Send logs before metrics to make sure logs are submitted even if the metrics output is not parsable.
	if err := k6LogsToLogger(result.Logs, logger); err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("cannot load logs to logger")
		return false, 0, err
	}

	var (
		collector         sampleCollector
		resultCollector   checkResultCollector
		durationCollector probeDurationCollector
	)

	if err := extractMetricSamples(result.Metrics, internalLogger, collector.process, resultCollector.process, durationCollector.process); err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("cannot extract metric samples")
		return false, 0, err
	}

	if err := registry.Register(&collector.collector); err != nil {
		internalLogger.Error().
			Err(err).
			Msg("cannot register collector")
		return false, 0, err
	}

	// https://github.com/grafana/sm-k6-runner/blob/b811839d444a7e69fd056b0a4e6ccf7e914197f3/internal/mq/runner.go#L51
	switch result.ErrorCode {
	case "":
		// No error, all good.
		return true, durationCollector.duration, nil
	// TODO: Remove "user" from this list, which has been renamed to "aborted".
	case "timeout", "killed", "user", "failed", "aborted":
		// These are user errors. The probe failed, but we don't return an error.
		return false, durationCollector.duration, nil
	default:
		// We got an "unknown" error, or some other code we do not recognize. Return it so we log it.
		return false, durationCollector.duration, fmt.Errorf("%w: %s: %s", ErrFromRunner, result.ErrorCode, result.Error)
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

type probeDurationCollector struct {
	duration time.Duration
}

func (dc *probeDurationCollector) process(_ *dto.MetricFamily, sample *model.Sample) error {
	if sample.Metric[model.MetricNameLabel] != "probe_script_duration_seconds" {
		return nil
	}

	dc.duration = time.Duration(float64(sample.Value) * float64(time.Second))

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
		var line []any
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

	return dec.Err()
}
