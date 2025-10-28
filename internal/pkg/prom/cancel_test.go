package prom

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

type alwaysRetryClient struct{}

func (alwaysRetryClient) StoreBytes(ctx context.Context, req []byte) error { return NewRecoverableError(errors.New("retry")) }
func (alwaysRetryClient) StoreStream(ctx context.Context, req io.Reader) error { return NewRecoverableError(errors.New("retry")) }
func (alwaysRetryClient) CountRetries(retries float64)                     {}

// Ensure backoff sleep respects context cancellation and returns quickly.
func TestSendBytesWithBackoff_RespectsContextCancellation(t *testing.T) {
	// Make backoff large to detect if cancellation is ignored.
	oldMin, oldMax := minBackoff, maxBackoff
	minBackoff, maxBackoff = 2*time.Second, 2*time.Second
	t.Cleanup(func() { minBackoff, maxBackoff = oldMin, oldMax })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(cancel)

	start := time.Now()
	err := SendBytesWithBackoff(ctx, alwaysRetryClient{}, []byte("hi"))
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected cancellation to be respected quickly, elapsed=%v", elapsed)
	}
}
