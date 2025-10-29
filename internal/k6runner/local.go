package k6runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

// secretSourceConfig represents the configuration for the secrets store
type secretSourceConfig struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

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

func (r Local) Run(ctx context.Context, script Script, secretStore SecretStore) (*RunResponse, error) {
	logger := r.logger.With().Object("checkInfo", &script.CheckInfo).Logger()

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
			logger.Error().Err(err).Str("severity", "critical").Msg("cannot remove temporary directory")
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

	var configFile string
	logger.Debug().
		Bool("secretStoreIsConfigured", secretStore.IsConfigured()).
		Str("secretStoreUrl", secretStore.Url).
		Bool("hasSecretStoreToken", secretStore.Token != "").
		Msg("checking secret store configuration")

	if secretStore.IsConfigured() {
		var cleanup func()
		configFile, cleanup, err = createSecretConfigFile(secretStore.Url, secretStore.Token)
		if err != nil {
			return nil, fmt.Errorf("cannot create secret config file: %w", err)
		}
		defer cleanup()

		logger.Debug().
			Str("secret_config_file", configFile).
			Str("secrets_url", secretStore.Url).
			Msg("Using secret config file")
	} else {
		logger.Warn().Msg("No secret store configuration available")
	}

	args, err := r.buildK6Args(script, metricsFn, logsFn, scriptFn, configFile)
	if err != nil {
		return nil, fmt.Errorf("building k6 arguments: %w", err)
	}

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
	cmd.Env = k6Env(os.Environ())

	start := time.Now()
	logger.Info().Str("command", cmd.String()).Msg("running k6 script")
	err = cmd.Run()

	duration := time.Since(start)

	// If context error is non-nil, incorporate it into err.
	// This brings context to log lines and plays well with both errors.Is and errors.As.
	err = errors.Join(err, ctx.Err())

	if err != nil && !isUserError(err) {
		// A non-user error occurred. This is usually something like k6 not being found, or other os-level errors trying
		// to run the binary. In this case, we don't bother to read k6 outputs and report the error immediately.
		logger.Error().
			Err(err).
			Dur("duration", duration).
			Msg("cannot run k6")

		dumpK6OutputStream(r.logger, zerolog.ErrorLevel, &stdout, "stream", "stdout")
		dumpK6OutputStream(r.logger, zerolog.ErrorLevel, &stderr, "stream", "stderr")

		logs, _ := afs.ReadFile(logsFn)
		dumpK6OutputStream(r.logger, zerolog.InfoLevel, bytes.NewReader(logs), "stream", "logs")

		return nil, fmt.Errorf("executing k6 script: %w", err)
	}

	// 256KiB is the maximum payload size for Loki. Set our limit slightly below that to avoid tripping the limit in
	// case we inject some messages down the line.
	const maxLogsSizeBytes = 255 * 1024

	// Mimir can also ingest up to 256KiB, but that's JSON-encoded, not promhttp encoded.
	// To be safe, we limit it to 100KiB promhttp-encoded, hoping than the more verbose json encoding overhead is less
	// than 2.5x.
	const maxMetricsSizeBytes = 100 * 1024

	logs, truncated, logsErr := readFileLimit(afs.Fs, logsFn, maxLogsSizeBytes)
	if logsErr != nil {
		return nil, fmt.Errorf("reading k6 logs: %w", err)
	}
	if truncated {
		logger.Warn().
			Str("filename", logsFn).
			Int("limitBytes", maxLogsSizeBytes).
			Msg("Logs output larger than limit, truncating")

		// Leave a truncation notice at the end.
		fmt.Fprintf(logs, `level=error msg="Log output truncated at %d bytes"`+"\n", maxLogsSizeBytes)
	}

	metrics, truncated, metricsErr := readFileLimit(afs.Fs, metricsFn, maxMetricsSizeBytes)
	if metricsErr != nil {
		return nil, fmt.Errorf("reading k6 metrics: %w", err)
	}
	if truncated {
		logger.Warn().
			Str("filename", metricsFn).
			Int("limitBytes", maxMetricsSizeBytes).
			Msg("Metrics output larger than limit, truncating")

		// If we truncate metrics, also leave a truncation notice at the end of the logs.
		fmt.Fprintf(logs, `level=error msg="Metrics output truncated at %d bytes"`+"\n", maxMetricsSizeBytes)
	}

	rr := &RunResponse{Metrics: metrics.Bytes(), Logs: logs.Bytes()}
	if err := errors.Join(err, errorFromLogs(logs.Bytes())); err != nil {
		// A user-error occurred: Either a context error, a k6 exit code we recognize as such, or an error was inferred
		// from the logs. In this case, absorb the error into the RunResponse so it can be reported back to the user.
		rr.ErrorCode = errorType(err)
		rr.Error = err.Error()
	}

	return rr, nil
}

func (r Local) buildK6Args(script Script, metricsFn, logsFn, scriptFn, configFile string) ([]string, error) {
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
		"--address", "", // Disable REST API server
		"--no-thresholds",
		"--no-usage-report",
		"--no-color",
		"--no-summary",
		"--verbose",
		"--throw", // Abort with an exception on certain abnormal cases: https://grafana.com/docs/k6/latest/using-k6/k6-options/reference/#throw
	}

	// Add secretStore configuration if available
	if configFile != "" {
		args = append(args, "--secret-source", "grafanasecrets=config="+configFile)
		if r.logger != nil {
			r.logger.Debug().
				Str("configFile", configFile).
				Msg("Adding secret source configuration to k6")
		}
	} else if r.logger != nil {
		r.logger.Debug().Msg("No secret source configuration to add to k6")
	}

	if script.CheckInfo.Type != synthetic_monitoring.CheckTypeBrowser.String() {
		args = append(args,
			"--vus", "1",
			"--iterations", "1",
		)
	}

	args = append(args, scriptFn)

	return args, nil
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

func dumpK6OutputStream(logger *zerolog.Logger, lvl zerolog.Level, stream io.Reader, fields ...any) {
	scanner := bufio.NewScanner(stream)

	for scanner.Scan() {
		logger.WithLevel(lvl).Fields(fields).Str("line", scanner.Text()).Msg("k6 output")
	}

	if err := scanner.Err(); err != nil {
		logger.Error().Fields(fields).Err(err).Msg("reading k6 output")
	}
}

// readFileLimit reads up to limit bytes from the specified file using the specified FS. The limit respects newline
// boundaries: If the limit is reached, the portion between the last '\n' character and the limit will not be returned.
// A boolean is returned indicating whether the limit was reached.
func readFileLimit(f afero.Fs, name string, limit int64) (*bytes.Buffer, bool, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, false, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	buf := &bytes.Buffer{}
	copied, err := io.Copy(buf, io.LimitReader(file, limit))
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, fmt.Errorf("reading file: %w", err)
	}

	if copied < limit {
		// Copied less than budget, we haven't truncated anything.
		return buf, false, nil
	}

	peek := make([]byte, 1)
	_, err = file.Read(peek)
	if errors.Is(err, io.EOF) {
		// Jackpot, file fit exactly within the limit.
		return buf, false, nil
	}

	// Rewind until last newline
	lastNewline := bytes.LastIndexByte(buf.Bytes(), '\n')
	if lastNewline != -1 {
		buf.Truncate(lastNewline + 1)
	}

	return buf, true, nil
}

// createSecretConfigFile creates a JSON config file with the given secret store URL and token
func createSecretConfigFile(url, token string) (filename string, cleanup func(), err error) {
	tmpFile, err := os.CreateTemp("", "k6-secrets-*.json")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp file: %w", err)
	}

	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		os.Remove(tmpFile.Name())
		return "", nil, fmt.Errorf("setting file permissions: %w", err)
	}

	config := secretSourceConfig{
		URL:   url,
		Token: token,
	}

	configData, err := json.Marshal(config)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", nil, fmt.Errorf("marshaling config to JSON: %w", err)
	}

	if _, err := tmpFile.Write(configData); err != nil {
		os.Remove(tmpFile.Name())
		return "", nil, fmt.Errorf("writing config file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", nil, fmt.Errorf("closing config file: %w", err)
	}

	return tmpFile.Name(), func() { os.Remove(tmpFile.Name()) }, nil
}
