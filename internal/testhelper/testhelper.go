package testhelper

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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
