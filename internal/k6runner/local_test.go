package k6runner

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/afero"
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

			if info.Mode().Perm() != 0o600 {
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

func TestReadFileLimit(t *testing.T) {
	t.Parallel()

	writeFile := func(fs afero.Fs, name, content string) {
		t.Helper()
		if err := afero.WriteFile(fs, name, []byte(content), 0o600); err != nil {
			t.Fatalf("writing test file: %v", err)
		}
	}

	tests := map[string]struct {
		fileContent   string
		limit         int64
		existing      string // pre-filled content in the buffer (not counted toward limit)
		wantContent   string
		wantTruncated bool
	}{
		"file fits within limit": {
			fileContent:   "line one\nline two\n",
			limit:         100,
			wantContent:   "line one\nline two\n",
			wantTruncated: false,
		},
		"file fits exactly at limit": {
			fileContent:   "line one\nline two\n", // 18 bytes
			limit:         18,
			wantContent:   "line one\nline two\n",
			wantTruncated: false,
		},
		"limit falls mid-line: partial line is dropped": {
			// limit falls inside "line three", so "line three\n" is dropped entirely
			fileContent:   "line one\nline two\nline three\n",
			limit:         20, // "line one\nline two\n" is 18 bytes; limit cuts 2 bytes into "line three"
			wantContent:   "line one\nline two\n",
			wantTruncated: true,
		},
		"large final line straddling limit is dropped, not truncated": {
			// Simulates a large httpDebug response line that pushes past the limit.
			// The preceding small lines should be kept; the large line should be dropped entirely.
			fileContent:   "small line one\nsmall line two\n" + strings.Repeat("x", 50),
			limit:         50, // cuts into the large line
			wantContent:   "small line one\nsmall line two\n",
			wantTruncated: true,
		},
		"pre-existing buffer content is not counted toward limit": {
			existing:      "pre-existing line\n",
			fileContent:   "file line one\nfile line two\n",
			limit:         100, // more than enough for the file content
			wantContent:   "pre-existing line\nfile line one\nfile line two\n",
			wantTruncated: false,
		},
		"pre-existing buffer content is not counted toward limit, file truncated": {
			existing:      "pre-existing line\n",
			fileContent:   "file line one\nfile line two\n",
			limit:         20, // cuts into "file line two\n"
			wantContent:   "pre-existing line\nfile line one\n",
			wantTruncated: true,
		},
		// edge case: when there is not a newline in the existing buffer, partial content is kept
		"no newline in existing: partial line is kept up to the limit": {
			existing:      "",
			fileContent:   "line one line two\n",
			limit:         5,
			wantContent:   "line ",
			wantTruncated: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fs := afero.NewMemMapFs()
			writeFile(fs, "test.log", tt.fileContent)

			existing := &bytes.Buffer{}
			existing.WriteString(tt.existing)

			got, truncated, err := readFileLimit(fs, "test.log", tt.limit, existing)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", truncated, tt.wantTruncated)
			}
			if got.String() != tt.wantContent {
				t.Errorf("content mismatch:\ngot:  %q\nwant: %q", got.String(), tt.wantContent)
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
