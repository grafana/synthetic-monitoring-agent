// Package recall implements methods to remember when was a check last executed.
package recall

import (
	"context"
	"time"
)

type Recaller interface {
	Remember(ctx context.Context, globalCheckID int64, forgetAfter time.Duration) error
	Recall(ctx context.Context, globalCheckID int64) (time.Time, error)
}

// NopRecaller is a no-op implementation of Recaller that always returns the Unix epoch as the last time a check was
// executed.
type NopRecaller struct{}

func (n NopRecaller) Remember(_ context.Context, _ int64, _ time.Duration) error { return nil }
func (n NopRecaller) Recall(_ context.Context, _ int64) (time.Time, error) {
	return time.Unix(0, 0), nil
}

var _ Recaller = NopRecaller{} // Build-time assert NopRecaller implements Recaller.
