package v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCondition(t *testing.T) {
	const fastTimeout = 50 * time.Millisecond
	const slowTimeout = 1000 * time.Millisecond

	var timeout = slowTimeout
	if testing.Short() {
		timeout = fastTimeout
	}

	t.Run("don't fire until signaled", func(t *testing.T) {
		c := newCondition()
		wait(t, &c, false, timeout)
	})
	t.Run("fire after signaled", func(t *testing.T) {
		c := newCondition()
		c.Signal()
		wait(t, &c, true, timeout)
	})
	t.Run("single fire", func(t *testing.T) {
		c := newCondition()
		c.Signal()
		c.Signal()
		wait(t, &c, true, timeout)
		wait(t, &c, false, timeout)
	})
	t.Run("single fire", func(t *testing.T) {
		c := newCondition()
		c.Signal()
		c.Signal()
		wait(t, &c, true, timeout)
		wait(t, &c, false, timeout)
	})
	t.Run("goroutine", func(t *testing.T) {
		c := newCondition()
		go func() {
			c.Signal()
		}()
		wait(t, &c, true, timeout)
	})
	t.Run("loop", func(t *testing.T) {
		const iterations = 100
		a, b := newCondition(), newCondition()
		go func() {
			for i := 0; i < iterations; i++ {
				a.Signal()
				wait(t, &b, true, timeout)
			}
		}()
		for i := 0; i < iterations; i++ {
			wait(t, &a, true, timeout)
			b.Signal()
		}
	})
}

func wait(t *testing.T, c *condition, fire bool, timeout time.Duration) {
	tm := time.NewTimer(timeout)
	defer tm.Stop()
	fired := false
	select {
	case <-tm.C:
	case <-c.C():
		fired = true
	}
	require.Equal(t, fire, fired)
}
