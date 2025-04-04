package k6runner

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
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
		wantArgs      []string
	}{
		"script without secrets": {
			metricsFn:     "/tmp/metrics.json",
			logsFn:        "/tmp/logs.log",
			scriptFn:      "/tmp/script.js",
			blacklistedIP: "127.0.0.1",
			configFile:    "",
			wantArgs: []string{
				"--out", "sm=/tmp/metrics.json",
				"--log-output", "file=/tmp/logs.log",
				"--blacklist-ip", "127.0.0.1",
			},
		},
		"script with secrets": {
			script:        Script{},
			metricsFn:     "/tmp/metrics.json",
			logsFn:        "/tmp/logs.log",
			scriptFn:      "/tmp/script.js",
			blacklistedIP: "127.0.0.1",
			configFile:    configFilename,
			wantArgs: []string{
				"--out", "sm=/tmp/metrics.json",
				"--log-output", "file=/tmp/logs.log",
				"--blacklist-ip", "127.0.0.1",
				"--secret-source", "grafanasecrets=config=" + configFilename,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := Local{blacklistedIP: tt.blacklistedIP}
			args, err := r.buildK6Args(tt.script, tt.metricsFn, tt.logsFn, tt.scriptFn, tt.configFile)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			for _, want := range tt.wantArgs {
				if !slices.Contains(args, want) {
					t.Errorf("buildK6Args() missing expected argument got \n%v\nwant \n%v", args, want)
				}
			}
		})
	}
}
