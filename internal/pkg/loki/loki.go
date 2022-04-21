package loki

import (
	"context"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/logproto"
	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/prom"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
)

// sendSamples to the remote storage with backoff for recoverable errors.
// TODO: Inject here the counter with the "tenantID" tag already filled
func SendStreamsWithBackoff(ctx context.Context, client *prom.Client, streams []logproto.Stream, buf *[]byte, retriesCtr pusher.RetriesCounter) error {
	req, err := buildStreamsPushRequest(streams, *buf)
	*buf = req
	if err != nil {
		// Failing to build the write request is non-recoverable, since it will
		// only error if marshaling the proto to bytes fails.
		return err
	}

	return prom.SendBytesWithBackoff(ctx, client, req, retriesCtr)
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
