package loki

import (
	"context"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/logproto"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/prom"
)

var minBackoff = 30 * time.Millisecond
var maxBackoff = 100 * time.Millisecond

type recoverableError struct {
	error
}

// sendSamples to the remote storage with backoff for recoverable errors.
func SendStreamsWithBackoff(ctx context.Context, client *prom.Client, streams []logproto.Stream, buf *[]byte) error {
	backoff := minBackoff
	req, err := buildStreamsPushRequest(streams, *buf)
	*buf = req
	if err != nil {
		// Failing to build the write request is non-recoverable, since it will
		// only error if marshaling the proto to bytes fails.
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := client.Store(ctx, req)

		if err == nil {
			return nil
		}

		if _, ok := err.(recoverableError); !ok {
			return err
		}

		time.Sleep(backoff)
		backoff = backoff * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func buildStreamsPushRequest(streams []logproto.Stream, buf []byte) ([]byte, error) {
	req := &logproto.PushRequest{
		Streams: streams,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	// snappy uses len() to see if it needs to allocate a new slice. Make the
	// buffer as long as possible.
	if buf != nil {
		buf = buf[0:cap(buf)]
	}
	compressed := snappy.Encode(buf, data)
	return compressed, nil
}
