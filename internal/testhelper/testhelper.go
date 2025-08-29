package testhelper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func Context(ctx context.Context, t *testing.T) (context.Context, context.CancelFunc) {
	deadline, found := t.Deadline()
	if !found {
		deadline = time.Now().Add(30 * time.Second)
	}

	return context.WithDeadline(ctx, deadline)
}

func Logger(t *testing.T) zerolog.Logger {
	logger := zerolog.New(zerolog.NewTestWriter(t)).Level(zerolog.ErrorLevel)
	if testing.Verbose() {
		logger = logger.Level(zerolog.DebugLevel)
	}

	return logger.With().Caller().Timestamp().Logger()
}

func MustReadFile(t *testing.T, filename string) []byte {
	t.Helper()

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	return data
}

func ModuleDir(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		// uh?
		return ""
	}
	dir := filepath.Dir(filename)
	for dir != "/" {
		gomod := filepath.Join(dir, "go.mod")
		_, err := os.Stat(gomod)
		switch {
		case err == nil:
			return dir
		case os.IsNotExist(err):
			dir = filepath.Join(dir, "..")
			continue
		default:
			panic(err)
		}
	}

	return dir
}

func K6Path(t *testing.T) string {
	t.Helper()

	k6path := filepath.Join(ModuleDir(t), "dist", runtime.GOOS+"-"+runtime.GOARCH, "sm-k6")
	require.FileExistsf(t, k6path, "k6 program must exist at %s", k6path)

	return k6path
}

// NoopSecretStore is a test implementation of the SecretProvider interface
// that returns an empty secret store. Use this in tests when you need a
// secret store but don't care about the actual secrets.
type NoopSecretStore struct{}

func (n NoopSecretStore) GetSecretCredentials(ctx context.Context, tenantID model.GlobalID) (*sm.SecretStore, error) {
	return &sm.SecretStore{}, nil
}

func (n NoopSecretStore) GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
	return "", nil
}

// TestSecretStore is a test implementation of the SecretProvider interface
// that returns a mock secret store with test credentials. Use this in tests
// when you need to test behavior that depends on having actual secret values.
type TestSecretStore struct{}

func (s TestSecretStore) GetSecretCredentials(ctx context.Context, tenantId model.GlobalID) (*sm.SecretStore, error) {
	if tenantId == 0 {
		return nil, errors.New("invalid tenant ID")
	}

	return &sm.SecretStore{
		Url:   "http://example.com",
		Token: "test-token",
	}, nil
}

func (s TestSecretStore) GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error) {
	if tenantID == 0 {
		return "", errors.New("invalid tenant ID")
	}

	// For testing purposes, return a mock secret value
	return "test-secret-value", nil
}
