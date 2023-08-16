package v2

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/prom"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type queue struct {
	options   *pusherOptions
	dataMutex sync.Mutex
	data      []queueEntry
	pending   condition
}

func newQueue(options *pusherOptions) queue {
	return queue{
		options: options,
		pending: newCondition(),
	}
}

func (q *queue) push(ctx context.Context, remote *sm.RemoteInfo) error {
	cfg, err := pusher.ClientFromRemoteInfo(remote)
	if err != nil {
		return err
	}
	client, err := prom.NewClient(remote.Name, cfg, unusedRetryCounterFn)
	if err != nil {
		q.options.logger.Error().Err(err).Msg("get client failed")
		q.options.metrics.FailedCounter.WithLabelValues(pusher.LabelValueClient).Inc()
		return pushError{
			kind:  errKindFatal,
			inner: fmt.Errorf("creating client: %w", err),
		}
	}

	var (
		retries  = q.options.retriesCounter()
		backoff  = q.options.backOffer()
		retrying = false
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-q.WaitC():
			records := q.get()
			if len(records) == 0 {
				continue
			}

			numRecords := float64(len(records))

			if !retrying {
				retries.reset()
				backoff.reset()
			}
			retrying = false

			concatReader := newConcatReader(records)
			httpStatusCode, pushErr := parsePublishError(client.StoreStream(ctx, &concatReader))
			statusCodeStr := strconv.Itoa(httpStatusCode)

			// TODO(mem): this might be a problem, we are keeping a
			// tally of the status codes received for each
			// individual tenant. It might be better to send this
			// to logs.
			q.options.metrics.ResponseCounter.WithLabelValues(statusCodeStr).Inc()

			if pushErr.IsRetriable() {
				q.options.metrics.ErrorCounter.WithLabelValues(statusCodeStr).Add(numRecords)
				if retrying = retries.retry(); retrying {
					q.options.metrics.RetriesCounter.WithLabelValues().Add(numRecords)
					// This causes each retry to use all pending records (up to maxPushBytes).
					// Is this what we want?
					// Conceptually it can make more sense to always retry with the same packet?
					// In the case where something in the packet is causing the 500
					q.requeue(records)
					if err := backoff.wait(ctx); err != nil {
						return err // ctx was cancelled
					}
					continue
				}
			}

			size := q.options.pool.returnAll(records)

			switch pushErr.Kind() {
			case errKindNoError:
				q.options.metrics.PushCounter.WithLabelValues().Add(numRecords)
				q.options.metrics.BytesOut.WithLabelValues().Add(float64(size))
				continue

			case errKindNetwork:
				q.options.metrics.FailedCounter.WithLabelValues(pusher.LabelValueRetryExhausted).Add(numRecords)

			case errKindPayload:
				// This is not necessarily errors! Possibly most of the data was ingested and only
				// a sample was discarded.
				q.options.metrics.ErrorCounter.WithLabelValues(statusCodeStr).Add(numRecords)

			case errKindTenant, errKindFatal, errKindWait:
				// Terminate publisher.
				q.options.metrics.ErrorCounter.WithLabelValues(statusCodeStr).Add(numRecords)
				return pushErr

			case errKindTerminated:
				// This can't really happen as client.StoreStream uses context.Background with a timeout.
				return pushErr

			default:
				// What kind of error is this?
				panic(pushErr)
			}
		}
	}
}

func (q *queue) insert(data *[]byte) {
	q.dataMutex.Lock()
	defer q.dataMutex.Unlock()
	q.data = append(q.data, queueEntry{
		data: data,
		ts:   time.Now(),
	})
	q.applyLimits()
	q.pending.Signal()
}

func (q *queue) applyLimits() {
	numDropped := q.limitNumItems(q.options.maxQueuedItems) + q.limitBytes(q.options.maxQueuedBytes) + q.limitAge(q.options.maxQueuedTime)
	if numDropped > 0 {
		q.options.metrics.DroppedCounter.WithLabelValues().Add(float64(numDropped))
	}
}

func (q *queue) limitNumItems(max int) (numRemoved int) {
	var (
		n      = len(q.data)
		excess = n - max
	)
	if max <= 0 || excess <= 0 {
		return 0
	}
	q.options.pool.returnAll(q.data[:excess])
	q.data = q.data[excess:]
	return excess
}

func (q *queue) limitBytes(max uint64) (numRemoved int) {
	n := len(q.data)
	if max <= 0 {
		return 0
	}
	for i, numBytes := n-1, uint64(0); i >= 0; i-- {
		if numBytes += uint64(len(*q.data[i].data)); numBytes > max {
			q.options.pool.returnAll(q.data[:i+1])
			q.data = q.data[i+1:]
			return i + 1
		}
	}
	return 0
}

func (q *queue) limitAge(maxAge time.Duration) (numRemoved int) {
	n := len(q.data)
	if n == 0 || maxAge <= 0 {
		return 0
	}
	limit := time.Now().Add(-maxAge)
	if !q.data[0].ts.Before(limit) {
		return 0
	}

	// As data is chronologically sorted (q.data[i].ts <= q.data[i+1].ts)
	// find all items older than limit using binary search.
	idx := sort.Search(n, func(i int) bool {
		return !q.data[i].ts.Before(limit)
	})
	if idx < n {
		q.options.pool.returnAll(q.data[:idx])
		q.data = q.data[idx:]
	}
	return n - idx
}

func (q *queue) WaitC() <-chan struct{} {
	return q.pending.C()
}

func (q *queue) get() []queueEntry {
	q.dataMutex.Lock()
	defer q.dataMutex.Unlock()

	totalQueued := len(q.data)
	if totalQueued == 0 {
		return nil
	}

	limit, numBytes := 1, uint64(len(*q.data[0].data))
	for limit < totalQueued {
		thisSize := uint64(len(*q.data[limit].data))
		if numBytes+thisSize > q.options.maxPushBytes {
			break
		}
		numBytes += thisSize
		limit++
	}

	// Copy the entries to return out of the slice, and move the remaining
	// entries to the front of the slice. In this way we avoid growing the
	// slice in case we add new data.
	take := make([]queueEntry, limit)
	copy(take, q.data[:limit])
	copy(q.data, q.data[limit:])
	q.data = q.data[:len(q.data[limit:])]

	if limit < totalQueued {
		q.pending.Signal()
	}

	return take
}

func (q *queue) requeue(data []queueEntry) {
	if len(data) == 0 {
		return
	}
	q.dataMutex.Lock()
	defer q.dataMutex.Unlock()
	q.data = append(data, q.data...)
	q.applyLimits()
	q.pending.Signal()
}

type queueEntry struct {
	data *[]byte
	ts   time.Time
}

func newConcatReader(records []queueEntry) SnappyConcatReader {
	streams := make([][]byte, len(records))
	for idx, rec := range records {
		streams[idx] = *rec.data
	}
	return SnappyConcatReader{
		Streams: streams,
	}
}

func unusedRetryCounterFn(float64) {}
