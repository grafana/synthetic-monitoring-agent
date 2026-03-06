package k6version_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6version"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

type fakeRunner struct {
	versionsCh <-chan []string
}

var _ k6runner.Runner = (*fakeRunner)(nil)

func (r *fakeRunner) WithLogger(_ *zerolog.Logger) k6runner.Runner { return r }

func (r *fakeRunner) Run(_ context.Context, _ k6runner.Script, _ k6runner.SecretStore) (*k6runner.RunResponse, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeRunner) Versions(_ context.Context) <-chan []string {
	return r.versionsCh
}

type fakeK6Client struct {
	err   error
	code  sm.StatusCode
	calls chan *sm.RegisterK6VersionRequest
}

var _ sm.K6Client = (*fakeK6Client)(nil)

func (c *fakeK6Client) RegisterK6Version(ctx context.Context, req *sm.RegisterK6VersionRequest, _ ...grpc.CallOption) (*sm.RegisterK6VersionResponse, error) {
	if c.calls != nil {
		select {
		case c.calls <- req:
		default:
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.err != nil {
		return nil, c.err
	}
	return &sm.RegisterK6VersionResponse{
		Status: sm.Status{Code: c.code},
	}, nil
}

//nolint:gocyclo // Test suite.
func TestHandle(t *testing.T) {
	t.Parallel()

	t.Run("cancelled context", func(t *testing.T) {
		t.Parallel()

		ch := make(chan []string) // never sends
		defer close(ch)

		logger := zerolog.Nop()

		handler, err := k6version.NewHandler(k6version.HandlerOpts{
			K6Runner: &fakeRunner{versionsCh: ch},
			K6Client: &fakeK6Client{code: sm.StatusCode_OK},
			Logger:   &logger,
		})
		if err != nil {
			t.Fatalf("creating handler: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		t.Cleanup(cancel)

		err = handler.Handle(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected %v, got %v", context.DeadlineExceeded, err)
		}
	})

	t.Run("closed versions channel", func(t *testing.T) {
		t.Parallel()

		ch := make(chan []string)
		close(ch)

		logger := zerolog.Nop()

		handler, err := k6version.NewHandler(k6version.HandlerOpts{
			K6Runner: &fakeRunner{versionsCh: ch},
			K6Client: &fakeK6Client{code: sm.StatusCode_OK},
			Logger:   &logger,
		})
		if err != nil {
			t.Fatalf("creating handler: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.Cleanup(cancel)

		err = handler.Handle(ctx)
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("reports versions", func(t *testing.T) {
		t.Parallel()

		versionsCh := make(chan []string, 1)
		versionsCh <- []string{"1.2.3", "2.0.0"}

		calls := make(chan *sm.RegisterK6VersionRequest, 1)
		logger := zerolog.Nop()

		handler, err := k6version.NewHandler(k6version.HandlerOpts{
			K6Runner: &fakeRunner{versionsCh: versionsCh},
			K6Client: &fakeK6Client{code: sm.StatusCode_OK, calls: calls},
			Logger:   &logger,
		})
		if err != nil {
			t.Fatalf("creating handler: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		handleDone := make(chan error, 1)
		go func() {
			handleDone <- handler.Handle(ctx)
		}()

		var req *sm.RegisterK6VersionRequest
		select {
		case <-time.After(3 * time.Second):
			t.Fatalf("RegisterK6Version was not called within timeout")
		case req = <-calls:
		}

		cancel()

		select {
		case err := <-handleDone:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected %v, got %v", context.Canceled, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("Handle did not return after context cancellation")
		}

		if len(req.Versions) != 2 {
			t.Fatalf("expected 2 versions, got %d: %v", len(req.Versions), req.Versions)
		}

		got := map[string]bool{}
		for _, v := range req.Versions {
			got[v.Version] = true
		}

		for _, expected := range []string{"1.2.3", "2.0.0"} {
			if !got[expected] {
				t.Fatalf("expected version %q in request, got: %v", expected, req.Versions)
			}
		}
	})

	t.Run("reports two successive version updates", func(t *testing.T) {
		t.Parallel()

		versionsCh := make(chan []string)
		calls := make(chan *sm.RegisterK6VersionRequest, 2)
		logger := zerolog.Nop()

		handler, err := k6version.NewHandler(k6version.HandlerOpts{
			K6Runner: &fakeRunner{versionsCh: versionsCh},
			K6Client: &fakeK6Client{code: sm.StatusCode_OK, calls: calls},
			Logger:   &logger,
		})
		if err != nil {
			t.Fatalf("creating handler: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		handleDone := make(chan error, 1)
		go func() {
			handleDone <- handler.Handle(ctx)
		}()

		// Send first version update and wait for it to be reported.
		select {
		case versionsCh <- []string{"1.0.0"}:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out sending first version update")
		}

		var req1 *sm.RegisterK6VersionRequest
		select {
		case req1 = <-calls:
		case <-time.After(3 * time.Second):
			t.Fatal("RegisterK6Version was not called for first update within timeout")
		}

		// Send second version update and wait for it to be reported.
		select {
		case versionsCh <- []string{"2.0.0"}:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out sending second version update")
		}

		var req2 *sm.RegisterK6VersionRequest
		select {
		case req2 = <-calls:
		case <-time.After(3 * time.Second):
			t.Fatal("RegisterK6Version was not called for second update within timeout")
		}

		cancel()

		select {
		case err := <-handleDone:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected %v, got %v", context.Canceled, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Handle did not return after context cancellation")
		}

		if len(req1.Versions) != 1 || req1.Versions[0].Version != "1.0.0" {
			t.Fatalf("unexpected first request versions: %v", req1.Versions)
		}
		if len(req2.Versions) != 1 || req2.Versions[0].Version != "2.0.0" {
			t.Fatalf("unexpected second request versions: %v", req2.Versions)
		}
	})

	t.Run("retries on client error", func(t *testing.T) {
		t.Parallel()

		versionsCh := make(chan []string, 1)
		versionsCh <- []string{"1.2.3"}

		calls := make(chan *sm.RegisterK6VersionRequest, 10)
		logger := zerolog.Nop()

		handler, err := k6version.NewHandler(k6version.HandlerOpts{
			K6Runner: &fakeRunner{versionsCh: versionsCh},
			K6Client: &fakeK6Client{err: errors.New("API unavailable"), calls: calls},
			Logger:   &logger,
		})
		if err != nil {
			t.Fatalf("creating handler: %v", err)
		}

		// The first call fails at t=0, backoff is 1s, so a 2s timeout guarantees at least 2 calls.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		t.Cleanup(cancel)

		err = handler.Handle(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected %v, got %v", context.DeadlineExceeded, err)
		}

		if got := len(calls); got < 2 {
			t.Fatalf("expected at least 2 calls to RegisterK6Version, got %d", got)
		}
	})
}
