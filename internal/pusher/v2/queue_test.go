package v2

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

func TestQueue(t *testing.T) {
	const timeout = time.Second * 5
	defaultOptions := pusherOptions{
		maxPushBytes:   1024 * 1024,
		maxQueuedBytes: 0,
		maxQueuedItems: 0,
		maxQueuedTime:  0,
	}

	for title, tc := range map[string]struct {
		actions      []testAction
		options      *pusherOptions
		countDropped int
	}{
		"single": {
			actions: []testAction{
				insert(30),
				expect(timeout, []int{30}),
			},
		},
		"get all": {
			actions: []testAction{
				insert(10),
				insert(20),
				insert(30),
				expect(timeout, []int{10, 20, 30}),
			},
		},
		"multiple operations": {
			actions: []testAction{
				insert(10),
				insert(20),
				insert(30),
				expect(timeout, []int{10, 20, 30}),
				insert(40),
				insert(50),
				expect(timeout, []int{40, 50}),
			},
		},
		"max bytes": {
			options: &pusherOptions{
				maxPushBytes: 30,
			},
			actions: []testAction{
				insert(30),
				insert(10),
				expect(timeout, []int{30}),
				insert(20),
				insert(30),
				expect(timeout, []int{10, 20}),
				insert(30),
				expect(timeout, []int{30}),
				expect(timeout, []int{30}),
			},
		},
		"take at least one": {
			options: &pusherOptions{
				maxPushBytes: 1,
			},
			actions: []testAction{
				insert(100),
				insert(5),
				insert(5),
				expect(timeout, []int{100}),
				expect(timeout, []int{5}),
				expect(timeout, []int{5}),
			},
		},
		"empty": {
			actions: []testAction{
				expectEmpty(),
			},
		},
		"requeue": {
			actions: []testAction{
				insert(10),
				insert(20),
				expect(timeout, []int{10, 20}),
				insert(30),
				returnLast(),
				expect(timeout, []int{10, 20, 30}),
				expectEmpty(),
			},
		},
		"max queued bytes": {
			options: &pusherOptions{
				maxPushBytes:   1024,
				maxQueuedBytes: 20,
			},
			actions: []testAction{
				insert(9),
				insert(11),
				expect(timeout, []int{9, 11}),
				insert(10),
				insert(15),
				insert(5),
				expect(timeout, []int{15, 5}),
				expectEmpty(),
			},
			countDropped: 1,
		},
		"max queued bytes return last": {
			options: &pusherOptions{
				maxPushBytes:   1024,
				maxQueuedBytes: 30,
			},
			actions: []testAction{
				insert(2),
				insert(9),
				insert(11),
				expect(timeout, []int{2, 9, 11}),
				insert(10),
				insert(5),
				returnLast(),
				expect(timeout, []int{11, 10, 5}),
				expectEmpty(),
			},
			countDropped: 2,
		},
		"max queued items": {
			options: &pusherOptions{
				maxPushBytes:   1024,
				maxQueuedItems: 2,
			},
			actions: []testAction{
				insert(9),
				insert(11),
				expect(timeout, []int{9, 11}),
				insert(10),
				insert(11),
				insert(15),
				insert(5),
				insert(1),
				expect(timeout, []int{5, 1}),
				expectEmpty(),
			},
			countDropped: 3,
		},
		"max queued items return last": {
			options: &pusherOptions{
				maxPushBytes:   1024,
				maxQueuedItems: 3,
			},
			actions: []testAction{
				insert(1),
				insert(2),
				expect(timeout, []int{1, 2}),
				insert(3),
				insert(4),
				returnLast(),
				expect(timeout, []int{2, 3, 4}),
				expectEmpty(),
			},
			countDropped: 1,
		},
		"max queued time": {
			options: &pusherOptions{
				maxQueuedTime: 100 * time.Millisecond,
			},
			actions: []testAction{
				insert(1),
				insert(2),
				sleep(150 * time.Millisecond),
				insert(3),
				expect(timeout, []int{3}),
				expectEmpty(),
			},
			countDropped: 1,
		},
	} {
		t.Run(title, func(t *testing.T) {
			dropped := prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: "dropped",
			}, nil)
			if tc.options == nil {
				tc.options = new(pusherOptions)
				*tc.options = defaultOptions
			}
			tc.options.metrics.DroppedCounter = dropped
			var st testSavedState
			q := newQueue(tc.options)

			for _, action := range tc.actions {
				action(t, &q, &st)
			}
			m := dto.Metric{}
			require.NoError(t, dropped.WithLabelValues().Write(&m))
			require.NotNil(t, m.Counter)
			require.NotNil(t, m.Counter.Value)
			require.Equal(t, float64(tc.countDropped), *m.Counter.Value)
		})
	}
}

func TestQueuePush(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	body := "HELLO WORLD!"
	records := makeRecords([][]byte{
		snap(body[:6]),
		snap(body[6:]),
	})
	respond := func(code int, body string) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}
	}
	for title, tc := range map[string]struct {
		options     *pusherOptions
		responses   []http.HandlerFunc
		expectedErr error
	}{
		"200": {
			responses: []http.HandlerFunc{
				respond(http.StatusOK, "OK"),
			},
		},
		"500, 200": {
			responses: []http.HandlerFunc{
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusOK, "OK"),
			},
		},
		"500x2, 200": {
			responses: []http.HandlerFunc{
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusOK, "OK"),
			},
		},
		"more 500": {
			responses: []http.HandlerFunc{
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusOK, "OK"),
			},
			expectedErr: nil,
		},
		"tenant error": {
			responses: []http.HandlerFunc{
				respond(http.StatusUnauthorized, "invalid token"),
			},
			expectedErr: pushError{
				kind:  errKindTenant,
				inner: errors.New("server returned HTTP status 401 Unauthorized: invalid token"),
			},
		},
		"max retries": {
			options: func() *pusherOptions {
				opt := defaultPusherOptions
				opt.maxRetries = 3
				return &opt
			}(),
			responses: []http.HandlerFunc{
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
				respond(http.StatusInternalServerError, "error"),
			},
		},
		"discard": {
			responses: []http.HandlerFunc{
				respond(http.StatusBadRequest, "some data is bad"),
			},
		},
		"rate limit": {
			responses: []http.HandlerFunc{
				respond(http.StatusTooManyRequests, "Too many requests"),
			},
			expectedErr: pushError{
				kind:  errKindWait,
				inner: errors.New(`server returned HTTP status 429 Too Many Requests: Too many requests`),
			},
		},
		"fatal error": {
			responses: []http.HandlerFunc{
				respond(http.StatusTooManyRequests, "Maximum active stream limit exceeded"),
			},
			expectedErr: pushError{
				kind:  errKindFatal,
				inner: errors.New(`server returned HTTP status 429 Too Many Requests: Maximum active stream limit exceeded`),
			},
		},
	} {
		t.Run(title, func(t *testing.T) {
			srv := testServer{
				responses: tc.responses,
			}
			srv.start()
			defer srv.stop()
			url, err := url.Parse(srv.server.URL)
			require.NoError(t, err)

			registry := prometheus.NewRegistry()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
			defer cancel()

			opt := defaultPusherOptions
			if tc.options != nil {
				opt = *tc.options
			}
			opt.metrics = pusher.NewMetrics(registry)
			opt = opt.withTenant(1).withType("test")

			q := newQueue(&opt)
			for _, r := range records {
				q.insert(r.data)
			}

			errGroup, gCtx := errgroup.WithContext(ctx)
			errGroup.Go(func() error {
				return q.push(gCtx, &sm.RemoteInfo{
					Name: "test",
					Url:  url.String(),
				})
			})
			errGroup.Go(func() error {
				defer cancel()
				for {
					if err := sleepCtx(gCtx, time.Second); err != nil {
						return nil
					}
					if srv.done() {
						return nil
					}
				}
			})

			if err = errGroup.Wait(); err == context.Canceled {
				err = nil
			}

			if tc.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, err.Error(), tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
			require.True(t, srv.done())
			require.Equal(t, []byte(body), srv.receivedBody)
			require.Empty(t, q.get())
		})
	}
}

type testSavedState struct {
	lastGet []queueEntry
}

type testAction func(*testing.T, *queue, *testSavedState)

func insert(numBytes int) testAction {
	data := make([]byte, numBytes)
	return func(t *testing.T, q *queue, _ *testSavedState) {
		q.insert(&data)
	}
}

func expect(timeout time.Duration, expectedBlocks []int) testAction {
	return func(t *testing.T, q *queue, st *testSavedState) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-q.WaitC():
		}
		data := q.get()
		st.lastGet = data
		blocks := make([]int, len(data))
		for idx, d := range data {
			blocks[idx] = len(*d.data)
		}
		require.Equal(t, expectedBlocks, blocks)
	}
}

func expectEmpty() testAction {
	return func(t *testing.T, q *queue, st *testSavedState) {
		select {
		case <-q.WaitC():
			t.Fatal("pending")
		default:
		}
		require.Nil(t, q.get())
		select {
		case <-q.WaitC():
			t.Fatal("pending")
		default:
		}
	}
}

func returnLast() testAction {
	return func(t *testing.T, q *queue, st *testSavedState) {
		if st.lastGet == nil {
			t.Fatal(st.lastGet)
		}
		q.requeue(st.lastGet)
	}
}

func sleep(interval time.Duration) testAction {
	return func(t *testing.T, q *queue, st *testSavedState) {
		time.Sleep(interval)
	}
}
