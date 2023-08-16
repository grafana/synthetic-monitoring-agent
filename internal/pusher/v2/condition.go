package v2

// condition is a simple, channel-based condition variable.
type condition chan struct{}

// Signal signals the condition. Waking up a waiting goroutine.
func (c condition) Signal() {
	select {
	case c <- struct{}{}:
	default:
		// Already signaled
	}
}

// C returns the channel used for waiting.
func (c condition) C() <-chan struct{} {
	return c
}

func newCondition() condition {
	return make(chan struct{}, 1)
}
