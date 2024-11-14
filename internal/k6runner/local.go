package k6runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

type Local struct {
	k6path        string
	logger        *zerolog.Logger
	fs            afero.Fs
	blacklistedIP string
}

func (r Local) WithLogger(logger *zerolog.Logger) Runner {
	r.logger = logger
	return r
}

func (r Local) Run(ctx context.Context, script Script) (*RunResponse, error) {
	afs := afero.Afero{Fs: r.fs}

	checkTimeout := time.Duration(script.Settings.Timeout) * time.Millisecond
	if checkTimeout == 0 {
		return nil, ErrNoTimeout
	}

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

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	args := []string{
		"run",
		"--out", "sm=" + metricsFn,
		"--log-format", "logfmt",
		"--log-output", "file=" + logsFn,
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
	}

	if script.CheckInfo.Type != synthetic_monitoring.CheckTypeBrowser.String() {
		args = append(args,
			"--vus", "1",
			"--iterations", "1",
		)
	}

	args = append(args, scriptFn)

	cmd := exec.CommandContext(
		ctx,
		k6Path,
		args...,
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
