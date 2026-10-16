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
	"syscall"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner/version"
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

const (
	// k6CloudPushRefIDEnvVar is the name of the k6 runtime environment variable used to
	// tag browser tests as originated from Synthetic Monitoring. It contains a reference
	// to the check's execution ID. k6 will propagate this data to the window.k6 property,
	// which is parsed by FE O11y as a means to correlate between a specific browser session
	// and a check execution.
	k6CloudPushRefIDEnvVar = "K6_CLOUD_PUSH_REF_ID"

	// k6BrowserWSURLEnvVar is the k6 option, read from the process environment,
	// that makes the browser module connect to an already-running browser over
	// CDP instead of launching its own.
	k6BrowserWSURLEnvVar = "K6_BROWSER_WS_URL"

	// browserAcquireTimeoutCap is the maximum time to wait for a browser session
	// from the pool. See browserAcquireTimeout for the effective budget.
	browserAcquireTimeoutCap = 30 * time.Second
)

// secretSourceConfig represents the configuration for the secrets store
type secretSourceConfig struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type Local struct {
	repository    *version.Repository
	logger        *zerolog.Logger
	fs            afero.Fs
	blacklistedIP string
	// browserPool, if non-nil, provides remote browser sessions for browser
	// checks. When nil, k6 launches Chromium locally as usual.
	browserPool BrowserPool
}

func (r Local) WithLogger(logger *zerolog.Logger) Runner {
	r.logger = logger
	return r
}

func (r Local) Run(ctx context.Context, script Script, secretStore SecretStore, executionID string) (*RunResponse, error) {
	logger := r.logger.With().
		Object("checkInfo", &script.CheckInfo).
		Str("k6ChannelManifest", script.K6ChannelManifest).
		Logger()

	if script.K6ChannelManifest == "" {
		script.K6ChannelManifest = "*"

		logger.Warn().Msg("Script does not have a k6 channel assigned. Latest available version will be used.")
	}

	k6Version, err := r.repository.Resolve(script.K6ChannelManifest)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve k6 version: %w", err)
	}

	logger = logger.With().Str("k6Version", k6Version.Version.String()).Str("k6Path", k6Version.Path).Logger()

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

	// Only browser checks with a browser pool configured acquire a remote
	// session. For every other run this is a no-op: browserEnv is empty and
	// release does nothing.
	//
	// The acquire runs before the check timeout starts, so the wait does not
	// shorten the k6 run when ctx leaves room for it. If no session can be
	// acquired in time, the check fails.
	browserEnv, release, err := r.acquireBrowserSession(ctx, script, checkTimeout)
	if err != nil {
		return nil, err
	}
	// Fresh context: the check context may already be done when k6 exits.
	defer release(context.Background())

	var cancel context.CancelFunc

	ctx, cancel = context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	configFile, cleanupSecrets, err := secretConfigFile(logger, secretStore)
	if err != nil {
		return nil, err
	}
	defer cleanupSecrets()

	args, err := r.buildK6Args(script, k6Version.Version, metricsFn, logsFn, scriptFn, configFile, executionID)
	if err != nil {
		return nil, fmt.Errorf("building k6 arguments: %w", err)
	}

	cmd := exec.CommandContext(
		ctx,
		k6Version.Path,
		args...,
	)

	var stdout, stderr bytes.Buffer

	cmd.Dir = workdir
	cmd.Stdin = nil
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Env = append(k6Env(os.Environ()), browserEnv...)

	// k6 starts its own processes, such as a Chromium tree. Its own group lets one
	// signal stop all of them. k6 stops getting signals sent to the agent group, but
	// nothing needs that: systemd signals the cgroup, everything else the main PID.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Kill the full process group.
	cmd.Cancel = func() error { return killProcessGroup(cmd) }

	// Never let a stuck child block the agent. Wait() also waits for the output pipes,
	// which a leaked child holds open.
	cmd.WaitDelay = 5 * time.Second

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

	// Pre-fill logs buffer with k6 versioning info.
	logsBuf := &bytes.Buffer{}
	_, _ = fmt.Fprintf(
		logsBuf,
		`time=%q level=info k6ChannelManifest=%q k6version=%q k6Path=%q msg="Check will be run with resolved k6 version"`+"\n",
		start.Format(time.RFC3339Nano),
		script.K6ChannelManifest, k6Version.Version.String(), k6Version.Path,
	)

	logs, truncated, logsErr := readFileLimit(afs.Fs, logsFn, maxLogsSizeBytes, logsBuf)
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

	metrics, truncated, metricsErr := readFileLimit(afs.Fs, metricsFn, maxMetricsSizeBytes, &bytes.Buffer{})
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

// killProcessGroup sends SIGKILL to the whole process group that cmd leads.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}

	// A negative PID means the process group with this ID.
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)

	// Already gone. This keeps a stale error out of the check result.
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}

	return err
}

func (r Local) Versions(ctx context.Context) <-chan []string {
	ch := make(chan []string)

	go func() {
		// The list of k6 versions in the local repository is static: We will push to the channel at most once.
		defer close(ch)

		entries, err := r.repository.Entries()
		if err != nil {
			r.logger.Error().
				Err(err).
				Str("k6Repository", r.repository.Root).
				Str("k6RepositoryOverride", r.repository.Override).
				Msg("Could not retrieve k6 versions from repository")

			return
		}

		versions := make([]string, 0, len(entries))
		for _, e := range entries {
			versions = append(versions, e.Version.String())
		}

		select {
		case <-ctx.Done():
			r.logger.Error().Err(ctx.Err()).Msg("Aborting k6 version reporting")

		case ch <- versions:
		}
	}()

	return ch
}

// usesBrowserPool reports whether this run must acquire a remote browser
// from the pool: a pool is configured and the check is a browser check.
func (r Local) usesBrowserPool(script Script) bool {
	return r.browserPool != nil && script.CheckInfo.Type == synthetic_monitoring.CheckTypeBrowser.String()
}

// acquireBrowserSession acquires a remote browser session for a browser check
// running against a browser pool. It returns the environment that hands the
// session's CDP WebSocket URL to k6, and the release function to call once k6
// exits. When the run does not use the pool, the environment is empty and
// release is a no-op.
func (r Local) acquireBrowserSession(
	ctx context.Context, script Script, checkTimeout time.Duration,
) (env []string, release func(context.Context), err error) {
	if !r.usesBrowserPool(script) {
		return nil, func(context.Context) {}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, browserAcquireTimeout(ctx, checkTimeout))
	defer cancel()

	wsURL, release, err := r.browserPool.Acquire(ctx, script.CheckInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("acquiring browser session: %w", err)
	}

	return []string{k6BrowserWSURLEnvVar + "=" + wsURL}, release, nil
}

// browserAcquireTimeout returns how long to wait for a browser session: the time
// ctx leaves beyond the check timeout, so k6 keeps its full timeout, but at
// least half the check timeout and at most browserAcquireTimeoutCap.
func browserAcquireTimeout(ctx context.Context, checkTimeout time.Duration) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return browserAcquireTimeoutCap
	}

	spare := time.Until(deadline) - checkTimeout

	return min(browserAcquireTimeoutCap, max(spare, checkTimeout/2))
}

func (r Local) buildK6Args(script Script, k6Version *semver.Version, metricsFn, logsFn, scriptFn, configFile, executionID string) ([]string, error) {
	var (
		logger    *zerolog.Logger
		nopLogger = zerolog.Nop()
	)
	if r.logger != nil {
		logger = r.logger
	} else {
		logger = &nopLogger
	}

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
		"--summary-mode", "disabled",
		"--verbose",
		"--throw", // Abort with an exception on certain abnormal cases: https://grafana.com/docs/k6/latest/using-k6/k6-options/reference/#throw
	}

	// --log-ns-timestamps enables nanosecond precision timestamps in logs.
	// This option, introduced in v2.3.0, is only available in k6 2.0.0 and above.
	if k6Version.Major() >= 2 {
		args = append(args, "--log-ns-timestamps")
	}

	// Add secretStore configuration if available
	if configFile != "" {
		args = append(args, "--secret-source", "grafanasecrets=config="+configFile)
		logger.Debug().Str("configFile", configFile).Msg(
			"Adding secret source configuration to k6",
		)
	} else {
		logger.Debug().Msg("No secret source configuration to add to k6")
	}

	if script.CheckInfo.Type == synthetic_monitoring.CheckTypeBrowser.String() {
		if k6RefID, err := buildK6RefID(executionID); err != nil {
			logger.Warn().Err(err).Msg("error building k6RefID")
		} else {
			args = append(args, "-e", k6CloudPushRefIDEnvVar+"="+k6RefID)
		}
	} else {
		args = append(args,
			"--vus", "1",
			"--iterations", "1",
		)
	}

	args = append(args, scriptFn)

	return args, nil
}

// buildK6RefID builds the K6 Cloud Ref. ID by prefixing the
// Synthetic Monitoring check executionID with "sm:".
func buildK6RefID(executionID string) (string, error) {
	if executionID == "" {
		return "", fmt.Errorf("executionID is empty")
	}

	return "sm:" + executionID, nil
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
// existing must point to a bytes.Buffer instance, potentially with some data already in. This data is not accounted for
// in terms of limit.
func readFileLimit(f afero.Fs, name string, limit int64, existing *bytes.Buffer) (*bytes.Buffer, bool, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, false, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	copied, err := io.Copy(existing, io.LimitReader(file, limit))
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, fmt.Errorf("reading file: %w", err)
	}

	if copied < limit {
		// Copied less than budget, we haven't truncated anything.
		return existing, false, nil
	}

	peek := make([]byte, 1)

	_, err = file.Read(peek)
	if errors.Is(err, io.EOF) {
		// Jackpot, file fit exactly within the limit.
		return existing, false, nil
	}

	// Rewind until last newline
	lastNewline := bytes.LastIndexByte(existing.Bytes(), '\n')
	if lastNewline != -1 {
		existing.Truncate(lastNewline + 1)
	}

	return existing, true, nil
}

// secretConfigFile creates the k6 secret source config file when secretStore is
// configured. Otherwise it returns an empty path and a no-op cleanup, and k6
// runs without secrets.
func secretConfigFile(logger zerolog.Logger, secretStore SecretStore) (configFile string, cleanup func(), err error) {
	logger.Debug().
		Bool("secretStoreIsConfigured", secretStore.IsConfigured()).
		Str("secretStoreUrl", secretStore.Url).
		Bool("hasSecretStoreToken", secretStore.Token != "").
		Msg("checking secret store configuration")

	if !secretStore.IsConfigured() {
		logger.Warn().Msg("No secret store configuration available")

		return "", func() {}, nil
	}

	configFile, cleanup, err = createSecretConfigFile(secretStore.Url, secretStore.Token)
	if err != nil {
		return "", nil, fmt.Errorf("cannot create secret config file: %w", err)
	}

	logger.Debug().
		Str("secret_config_file", configFile).
		Str("secrets_url", secretStore.Url).
		Msg("Using secret config file")

	return configFile, cleanup, nil
}

// createSecretConfigFile creates a JSON config file with the given secret store URL and token
func createSecretConfigFile(url, token string) (filename string, cleanup func(), err error) {
	tmpFile, err := os.CreateTemp("", "k6-secrets-*.json")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp file: %w", err)
	}

	if err := os.Chmod(tmpFile.Name(), 0o600); err != nil {
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
