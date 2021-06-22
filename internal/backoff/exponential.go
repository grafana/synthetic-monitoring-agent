package backoff

import (
	"math/rand"
	"time"
)

// Binary implements an binary exponential backoff algorithm in the way
// Ethernet does it: progressively increase the the range from which the
// delay is chosen. The first time it's 0 or 1*base; the second time
// it's 0, 1*base or 2*base; the third 0, 1*base, 2*base or 4*base, etc.
type Binary struct {
	baseDelay time.Duration
	maxExp    int64
	c         int64
}

// NewBinary creates a new binary exponential backoff controller, using
// baseDelay as the base and maxExp as the maximum number of times the
// exponent is increased. After calling Get() that many times, the
// exponent is no longer increased, but the delay is still chosen at
// random from the possible values.
func NewBinary(baseDelay time.Duration, maxExp int64) Binary {
	return Binary{baseDelay: baseDelay, maxExp: maxExp}
}

// Get chooses a random delay from the possible set. Each time Get is
// called the maximum exponent is increased up to the specified limit.
func (b *Binary) Get() time.Duration {
	if b.c < b.maxExp {
		b.c++
	}

	return b.baseDelay * time.Duration(rand.Int63n(1<<b.c))
}

// Reset sets the controller back to the state it had after being
// created so the next time Get() is called it returns either 0 or
// 1*base.
func (b *Binary) Reset() {
	b.c = 0
}
