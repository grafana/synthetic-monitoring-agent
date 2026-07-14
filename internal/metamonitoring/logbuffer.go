package metamonitoring

import (
	"sync"
	"sync/atomic"

	logproto "github.com/grafana/loki/pkg/push"
)

// LogBuffer accumulates log streams between publish cycles.
// Implementations must be safe for concurrent use.
type LogBuffer interface {
	Append(logproto.Stream)
	Drain() []logproto.Stream
}

type RingBuffer struct {
	mu       sync.Mutex
	streams  []logproto.Stream
	maxItems int
	dropped  atomic.Int64
}

func NewRingBuffer(maxItems int) *RingBuffer {
	return &RingBuffer{
		streams:  make([]logproto.Stream, 0, maxItems),
		maxItems: maxItems,
	}
}

func (b *RingBuffer) Append(s logproto.Stream) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.streams) >= b.maxItems {
		b.streams = b.streams[1:]
		b.dropped.Add(1)
	}

	b.streams = append(b.streams, s)
}

func (b *RingBuffer) Drain() []logproto.Stream {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := b.streams
	b.streams = make([]logproto.Stream, 0, b.maxItems)

	return out
}

func (b *RingBuffer) Dropped() int64 {
	return b.dropped.Load()
}
