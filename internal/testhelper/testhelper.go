package testhelper

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Context(ctx context.Context, t *testing.T) (context.Context, context.CancelFunc) {
	deadline, found := t.Deadline()
	if !found {
		deadline = time.Now().Add(30 * time.Second)
	}

	return context.WithDeadline(ctx, deadline)
}

func MustReadFile(t *testing.T, filename string) []byte {
	t.Helper()

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	return data
}
