package multihttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-logfmt/logfmt"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/rs/zerolog"
)

type runner struct {
	script []byte
}

func newRunner(settings *sm.MultiHttpSettings) (*runner, error) {
	// convert settings to script

	script, err := settingsToScript(settings)
	if err != nil {
		return nil, fmt.Errorf("converting settings to script: %w", err)
	}

	r := runner{
		script: script,
	}

	return &r, nil
}

type requestError struct {
	Err     string `json:"error"`
	Message string `json:"msg"`
}

var _ error = requestError{}

func (r requestError) Error() string {
	return fmt.Sprintf("%s: %s", r.Err, r.Message)
}

type requestResult struct {
	Metrics []byte `json:"metrics"`
	Logs    []byte `json:"logs"`
}

func (r runner) Run(ctx context.Context, registry *prometheus.Registry, logger logger.Logger, internalLogger zerolog.Logger) error {
	resp, err := http.Post("http://localhost:4054/run", "application/json", bytes.NewBuffer(r.script))
	if err != nil {
		internalLogger.Error().Err(err).Msg("sending request")
		return fmt.Errorf("running script: %w", err)
	}

	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var result requestError

		err := dec.Decode(&result)
		if err != nil {
			internalLogger.Error().Err(err).Msg("decoding request response")
			return fmt.Errorf("running script: %w", err)
		}

		internalLogger.Error().Err(result).Msg("request response")
		return fmt.Errorf("running script: %w", result)
	}

	var result requestResult

	err = dec.Decode(&result)
	if err != nil {
		internalLogger.Error().Err(err).Msg("decoding script result")
		return fmt.Errorf("decoding script result: %w", err)
	}

	// here we have metrics in text format and logs in logfmt format.

	if err := textToRegistry(result.Metrics, registry, internalLogger); err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("cannot add metrics to registry")
		return err
	}

	if err := k6LogsToLogger(result.Logs, logger); err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("cannot load logs to logger")
		return err
	}

	return nil
}

func textToRegistry(metrics []byte, registry *prometheus.Registry, logger zerolog.Logger) error {
	promDecoder := expfmt.NewDecoder(bytes.NewBuffer(metrics), expfmt.FmtText)
	for {
		var mf dto.MetricFamily
		switch err := promDecoder.Decode(&mf); err {
		case nil:
			// Got one metric family
			samples, err := expfmt.ExtractSamples(nil, &mf)
			if err != nil {
				logger.Error().Err(err).Msg("extracting samples")
				return fmt.Errorf("extracting samples: %w", err)
			}

			for _, sample := range samples {
				// TODO(mem): This is really crappy. We have a
				// set of metric families, and we are
				// converting back to samples to that we can
				// add that to the registry. We need to rework
				// the logic in the prober so that it can
				// return a set of metric families. The probes
				// that don't have this, can create a registry
				// locally and get the metric families from
				// that.
				name, found := sample.Metric[model.MetricNameLabel]
				if !found {
					logger.Error().Err(err).Msg("missing metric name")
					return fmt.Errorf("missing metric name")
				}

				delete(sample.Metric, model.MetricNameLabel)

				g := prometheus.NewGauge(prometheus.GaugeOpts{
					Name:        string(name),
					ConstLabels: metricToLabels(sample.Metric),
				})

				err := registry.Register(g)
				if err != nil {
					return err
				}

				g.Set(float64(sample.Value))
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
		labels[string(name)] = string(value)
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

	for dec.ScanRecord() {
		var line []interface{}
		for dec.ScanKeyval() {
			key := dec.Key()
			value := dec.Value()
			line = append(line, string(key), string(value))
		}
		_ = logger.Log(line...)
	}

	return nil
}
