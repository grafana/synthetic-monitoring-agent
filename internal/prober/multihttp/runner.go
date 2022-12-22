package multihttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/go-logfmt/logfmt"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

type runner struct {
	script []byte
}

func newRunner(script []byte) (runner, error) {
	r := runner{
		script: script,
	}

	return r, nil
}

func (r runner) Run(ctx context.Context, registry *prometheus.Registry, logger logger.Logger, internalLogger zerolog.Logger) error {
	dir, err := os.MkdirTemp("", "*")
	if err != nil {
		return err
	}

	defer os.RemoveAll(dir)

	scriptFn, err := createTempFile("script", dir, "script-*.js", r.script)
	if err != nil {
		return err
	}

	summaryFn, err := createTempFile("summary", dir, "summary-*.json", nil)
	if err != nil {
		return err
	}

	logFn, err := createTempFile("log", dir, "log-*", nil)
	if err != nil {
		return err
	}

	// TODO(mem): figure out a way to run this process, possibly in a
	// sandbox or another container.
	//#nosec see above
	cmd := exec.CommandContext(
		ctx,
		"k6",
		"run",
		scriptFn,
		"--summary-export",
		summaryFn,
		"--iterations",
		"1",
		"--vus",
		"1",
		"--log-format=logfmt",
		"--log-output=file="+logFn,
		"--no-color",
		// "--no-summary",
		"--verbose",
		"--quiet",
	)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		internalLogger.Debug().
			Err(err).
			Str("stdout", stdout.String()).
			Str("stderr", stderr.String()).
			Msg("failed to execute multihttp")

		//  TODO: k6 is exiting every time with "signal: killed" even though it is running successfully (I think?)
		// 	return err
	}

	summaryBuf, err := os.ReadFile(summaryFn)
	if err != nil {
		internalLogger.Debug().
			Err(err).
			Str("filename", summaryFn).
			Msg("cannot read summary file")
		return err
	}

	logBuf, err := os.ReadFile(logFn)
	if err != nil {
		internalLogger.Debug().
			Err(err).
			Str("filename", logFn).
			Msg("cannot read log file")
		return err
	}

	internalLogger.Info().
		Bytes("summary", summaryBuf).
		Bytes("log", logBuf).
		Msg("script output")

	metrics, err := loadMetricsFromFile(summaryFn)
	if err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("cannot load metrics from file")
		return err
	}

	err = k6SummaryToRegistry(metrics, registry)
	if err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("cannot add metrics to registry")
		return err
	}

	err = k6LogsToLogger(logFn, logger)
	if err != nil {
		internalLogger.Debug().
			Err(err).
			Msg("cannot add metrics to registry")
		return err
	}

	return nil
}

func createTempFile(tag, dir, pattern string, content []byte) (string, error) {
	fh, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("creating temporary file for %s: %w", tag, err)
	}

	if len(content) > 0 {
		_, err = fh.Write(content)
		if err != nil {
			_ = fh.Close()
			return "", fmt.Errorf("writing %s to file %s: %w", tag, fh.Name(), err)
		}
	}

	err = fh.Close()
	if err != nil {
		return "", fmt.Errorf("closing file %s for %s: %w", fh.Name(), tag, err)
	}

	return fh.Name(), nil
}

type k6Summary struct {
	Metrics map[string]map[string]float64 `json:"metrics"`
}

func loadMetricsFromFile(fn string) (*k6Summary, error) {
	fh, err := os.Open(fn) //#nosec -- G304 -- FIXME(mem): input file needs to be sanitized?
	if err != nil {
		return nil, fmt.Errorf("opening file %s: %w", fn, err)
	}

	defer fh.Close() //#nosec -- G307 -- If we get an error from closing a RO file we have bigger problems

	var summary k6Summary

	err = json.NewDecoder(fh).Decode(&summary)
	if err != nil {
		return nil, fmt.Errorf("loading metrics from %s: %w", fn, err)
	}

	return &summary, nil
}

func k6SummaryToRegistry(summary *k6Summary, registry *prometheus.Registry) error {
	replacer := func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '_':
			return r
		case r == ':':
			return r
		default:
			return -1
		}
	}

	for family, metrics := range summary.Metrics {
		for name, value := range metrics {
			g := prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace: strings.Map(replacer, family),
				Name:      strings.Map(replacer, name),
			})
			err := registry.Register(g)
			if err != nil {
				return err
			}
			g.Set(value)
		}
	}

	return nil
}

func k6LogsToLogger(fn string, logger logger.Logger) error {
	// This seems a little silly, we should be able to take the out of k6
	// and pass it directly to Loki. The problem with that is that the only
	// thing probers have access to is the logger.Logger.
	//
	// We could pass another object to the prober, that would take Loki log
	// entries, and the publisher could decorate that with the necessary
	// labels.
	fh, err := os.Open(fn) //#nosec -- G304 -- this is ok, we chose the path
	if err != nil {
		return fmt.Errorf("opening file %s: %w", fn, err)
	}

	dec := logfmt.NewDecoder(fh)

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
