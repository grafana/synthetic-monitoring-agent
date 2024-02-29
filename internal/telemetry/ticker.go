package telemetry

import (
	"math/rand"
	"time"
)

const (
	jitterUpperBound = 60 // s
)

// ticker represents a ticker interface that
// can be mocked for more reliable tests.
type ticker interface {
	C() <-chan time.Time
	Stop()
}

func newStdTicker(d time.Duration) *stdTicker {
	return &stdTicker{
		Ticker: time.NewTicker(withJitter(d)),
	}
}

// stdTicker is a wrapper around the standard
// library time ticker.
type stdTicker struct {
	*time.Ticker
}

func (t *stdTicker) C() <-chan time.Time {
	return t.Ticker.C
}

func (t *stdTicker) Stop() {
	t.Ticker.Stop()
}

// withJitter sums a random jitter of [0, 59)s to the given duration.
// This will randomize the tenant pushers push time to avoid many overlapping
// requests to the API and instead distribute them more evenly.
func withJitter(d time.Duration) time.Duration {
	jitter := time.Duration(rand.Intn(jitterUpperBound)) * time.Second
	return d + jitter
}
