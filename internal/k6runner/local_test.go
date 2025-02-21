package k6runner

import (
	"encoding/base64"
	"os"
	"slices"
	"testing"
)

func TestCreateSecureTokenFile(t *testing.T) {
	tests := map[string]struct {
		tokenData string
	}{
		"valid token": {
			tokenData: "secret-token-123",
		},
		"empty token": {
			tokenData: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			filename, cleanup, err := createSecureTokenFile(tt.tokenData)
			defer cleanup()

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Check if file exists
			if _, err := os.Stat(filename); os.IsNotExist(err) {
				t.Error("token file was not created")
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
				t.Errorf("failed to read token file: %v", err)
				return
			}

			if string(content) != tt.tokenData {
				t.Errorf("expected token data %q, got %q", tt.tokenData, string(content))
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
	secretUrlBase64 := base64.URLEncoding.EncodeToString([]byte(secretUrl))
	tokenFilename, cleanup, err := createSecureTokenFile("secret-token")
	if err != nil {
		t.Fatalf("failed to create token file: %v", err)
	}
	defer cleanup()

	tests := map[string]struct {
		script        Script
		metricsFn     string
		logsFn        string
		scriptFn      string
		blacklistedIP string
		tokenFilename string
		wantArgs      []string
	}{
		"basic script without secrets": {
			metricsFn:     "/tmp/metrics.json",
			logsFn:        "/tmp/logs.log",
			scriptFn:      "/tmp/script.js",
			blacklistedIP: "127.0.0.1",
			tokenFilename: "",
			wantArgs: []string{
				"--out", "sm=/tmp/metrics.json",
				"--log-output", "file=/tmp/logs.log",
				"--blacklist-ip", "127.0.0.1",
			},
		},
		"script with secrets": {
			script: Script{
				SecretStore: SecretStore{
					Url:   secretUrl,
					Token: "secret-token",
				},
			},
			metricsFn:     "/tmp/metrics.json",
			logsFn:        "/tmp/logs.log",
			scriptFn:      "/tmp/script.js",
			blacklistedIP: "127.0.0.1",
			tokenFilename: tokenFilename,
			wantArgs: []string{
				"--out", "sm=/tmp/metrics.json",
				"--log-output", "file=/tmp/logs.log",
				"--blacklist-ip", "127.0.0.1",
				"--secret-source", "grafanasecrets=url_base64=" + secretUrlBase64 + ",token=" + tokenFilename,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := Local{blacklistedIP: tt.blacklistedIP}
			args, err := r.buildK6Args(tt.script, tt.metricsFn, tt.logsFn, tt.scriptFn, tt.tokenFilename)
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
