package k6runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateSecretConfigFile(t *testing.T) {
	tests := map[string]struct {
		url   string
		token string
	}{
		"valid data": {
			url:   "http://secrets.example.com",
			token: "secret-token-123",
		},
		"empty values": {
			url:   "",
			token: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			filename, cleanup, err := createSecretConfigFile(tt.url, tt.token)
			defer cleanup()

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Check if file exists
			if _, err := os.Stat(filename); os.IsNotExist(err) {
				t.Error("config file was not created")
				return
			}

			// Check file permissions
			info, err := os.Stat(filename)
			if err != nil {
				t.Errorf("failed to get file info: %v", err)
				return
			}

			if info.Mode().Perm() != 0600 {
				t.Errorf("expected file permissions 0600, got %v", info.Mode().Perm())
			}

			// Check file contents
			content, err := os.ReadFile(filename)
			if err != nil {
				t.Errorf("failed to read config file: %v", err)
				return
			}

			// Verify JSON format and content
			var config secretSourceConfig
			if err := json.Unmarshal(content, &config); err != nil {
				t.Errorf("failed to unmarshal JSON: %v", err)
				return
			}

			if config.URL != tt.url {
				t.Errorf("expected URL %q, got %q", tt.url, config.URL)
			}

			if config.Token != tt.token {
				t.Errorf("expected token %q, got %q", tt.token, config.Token)
			}

			// Test cleanup function
			cleanup()
			if _, err := os.Stat(filename); !os.IsNotExist(err) {
				t.Error("cleanup function did not remove the file")
			}
		})
	}
}

func TestBuildK6Args(t *testing.T) {
	secretUrl := "http://secrets.example.com"
	configFilename, cleanup, err := createSecretConfigFile(secretUrl, "secret-token")
	if err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}
	defer cleanup()

	tests := map[string]struct {
		script        Script
		metricsFn     string
		logsFn        string
		scriptFn      string
		blacklistedIP string
		configFile    string
		executionID   string
		wantArgs      []string
		wantAbsent    []string
	}{
		"script without secrets": {
			metricsFn:     "/tmp/metrics.json",
			logsFn:        "/tmp/logs.log",
			scriptFn:      "/tmp/script.js",
			blacklistedIP: "127.0.0.1",
			configFile:    "",
			executionID:   "test-exec-id",
			wantArgs: []string{
				"--out", "sm=/tmp/metrics.json",
				"--log-output", "file=/tmp/logs.log",
				"--blacklist-ip", "127.0.0.1",
				"--vus", "--iterations",
			},
			wantAbsent: []string{k6CloudPushRefIDEnvVar},
		},
		"script with secrets": {
			script:        Script{},
			metricsFn:     "/tmp/metrics.json",
			logsFn:        "/tmp/logs.log",
			scriptFn:      "/tmp/script.js",
			blacklistedIP: "127.0.0.1",
			configFile:    configFilename,
			executionID:   "test-exec-id",
			wantArgs: []string{
				"--out", "sm=/tmp/metrics.json",
				"--log-output", "file=/tmp/logs.log",
				"--blacklist-ip", "127.0.0.1",
				"--secret-source", "grafanasecrets=config=" + configFilename,
				"--vus", "--iterations",
			},
			wantAbsent: []string{k6CloudPushRefIDEnvVar},
		},
		"browser check sets K6_CLOUD_PUSH_REF_ID": {
			script: Script{
				CheckInfo: CheckInfo{Type: "browser"},
			},
			metricsFn:     "/tmp/metrics.json",
			logsFn:        "/tmp/logs.log",
			scriptFn:      "/tmp/script.js",
			blacklistedIP: "127.0.0.1",
			executionID:   "abc-123",
			wantArgs: []string{
				"-e", k6CloudPushRefIDEnvVar + "=sm:abc-123",
			},
			wantAbsent: []string{"--vus", "--iterations"},
		},
		"browser check with empty executionID omits K6_CLOUD_PUSH_REF_ID": {
			script: Script{
				CheckInfo: CheckInfo{Type: "browser"},
			},
			metricsFn:     "/tmp/metrics.json",
			logsFn:        "/tmp/logs.log",
			scriptFn:      "/tmp/script.js",
			blacklistedIP: "127.0.0.1",
			executionID:   "",
			wantAbsent:    []string{k6CloudPushRefIDEnvVar, "--vus", "--iterations"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := Local{blacklistedIP: tt.blacklistedIP}
			args, err := r.buildK6Args(tt.script, tt.metricsFn, tt.logsFn, tt.scriptFn, tt.configFile, tt.executionID)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			for _, want := range tt.wantArgs {
				if !slices.Contains(args, want) {
					t.Errorf("buildK6Args() missing expected argument got \n%v\nwant \n%v", args, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if slices.Contains(args, absent) {
					t.Errorf("buildK6Args() should not contain %q, got \n%v", absent, args)
				}
			}
		})
	}
}

func TestBuildK6RefID(t *testing.T) {
	t.Parallel()

	t.Run("valid executionID", func(t *testing.T) {
		got, err := buildK6RefID("abc-123-def")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "sm:abc-123-def" {
			t.Errorf("buildK6RefID() = %q, want %q", got, "sm:abc-123-def")
		}
	})

	t.Run("empty executionID returns error", func(t *testing.T) {
		_, err := buildK6RefID("")
		if err == nil {
			t.Fatal("expected error for empty executionID, got nil")
		}
	})
}

func TestLocalBrowserPool(t *testing.T) {
	t.Parallel()

	browserScript := func() Script {
		return Script{
			Script:            []byte("export default function() {}"),
			Settings:          Settings{Timeout: 5000},
			CheckInfo:         CheckInfo{Type: "browser", Metadata: map[string]any{"id": "123"}},
			K6ChannelManifest: "*",
		}
	}

	newLocalRunner := func(t *testing.T, pool BrowserPool) Runner {
		t.Helper()
		// The k6 fake is exec'd with the runner's temporary directory as
		// working directory, so its path must be absolute.
		k6Fake, err := filepath.Abs("./testdata/k6-env-fake")
		require.NoError(t, err)
		runner, err := New(RunnerOpts{Uri: k6Fake, BrowserPool: pool})
		require.NoError(t, err)
		return runner
	}

	t.Run("browser check acquires, injects and releases", func(t *testing.T) {
		t.Parallel()

		pool := &fakeBrowserPool{wsURL: "ws://pool-instance:8080/proxy/abc123"}
		runner := newLocalRunner(t, pool)

		rr, err := runner.Run(t.Context(), browserScript(), SecretStore{}, "exec-id")
		require.NoError(t, err)

		acquired, releases := pool.state()
		require.Len(t, acquired, 1)
		require.Equal(t, "browser", acquired[0].Type)
		require.Equal(t, 1, releases)
		// The fake k6 dumps K6_BROWSER_WS_URL into the logs: the session URL
		// reached the k6 process environment.
		require.Contains(t, string(rr.Logs), pool.wsURL)
	})

	t.Run("non-browser check does not use the pool", func(t *testing.T) {
		t.Parallel()

		pool := &fakeBrowserPool{wsURL: "ws://pool-instance:8080/proxy/abc123"}
		runner := newLocalRunner(t, pool)

		script := browserScript()
		script.CheckInfo.Type = "scripted"

		rr, err := runner.Run(t.Context(), script, SecretStore{}, "exec-id")
		require.NoError(t, err)

		acquired, releases := pool.state()
		require.Empty(t, acquired)
		require.Zero(t, releases)
		require.Contains(t, string(rr.Logs), "browserWsUrl=none")
	})

	t.Run("acquire error fails the check", func(t *testing.T) {
		t.Parallel()

		acquireErr := errors.New("browser pool exhausted")
		pool := &fakeBrowserPool{err: acquireErr}
		runner := newLocalRunner(t, pool)

		_, err := runner.Run(t.Context(), browserScript(), SecretStore{}, "exec-id")
		require.ErrorIs(t, err, acquireErr)

		_, releases := pool.state()
		require.Zero(t, releases)
	})

	t.Run("release on k6 failure", func(t *testing.T) {
		t.Parallel()

		pool := &fakeBrowserPool{wsURL: "ws://pool-instance:8080/proxy/abc123"}
		runner := newLocalRunner(t, pool)

		// The K6_FAKE_FAIL marker in the script makes the fake k6 exit 1,
		// which the runner classifies as a user error and absorbs into the
		// RunResponse.
		script := browserScript()
		script.Script = []byte("// K6_FAKE_FAIL")

		rr, err := runner.Run(t.Context(), script, SecretStore{}, "exec-id")
		require.NoError(t, err)
		require.NotEmpty(t, rr.Error)

		_, releases := pool.state()
		require.Equal(t, 1, releases)
	})

	t.Run("nil pool preserves local behavior", func(t *testing.T) {
		t.Parallel()

		runner := newLocalRunner(t, nil)

		rr, err := runner.Run(t.Context(), browserScript(), SecretStore{}, "exec-id")
		require.NoError(t, err)
		require.Contains(t, string(rr.Logs), "browserWsUrl=none")
	})
}

// fakeBrowserPool implements BrowserPool for tests.
type fakeBrowserPool struct {
	wsURL string
	err   error

	mtx      sync.Mutex
	acquired []CheckInfo
	releases int
}

func (f *fakeBrowserPool) Acquire(_ context.Context, checkInfo CheckInfo) (string, func(context.Context), error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	if f.err != nil {
		return "", nil, f.err
	}

	f.acquired = append(f.acquired, checkInfo)
	return f.wsURL, func(context.Context) {
		f.mtx.Lock()
		defer f.mtx.Unlock()
		f.releases++
	}, nil
}

func (f *fakeBrowserPool) state() (acquired []CheckInfo, releases int) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return slices.Clone(f.acquired), f.releases
}
