package loki

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	logproto "github.com/grafana/loki/pkg/push"
	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/prom"
)

// splitStreamsIntoChunks splits a slice of streams into chunks that fit within maxBytes.
// It respects stream boundaries and ensures each chunk is a valid PushRequest.
func splitStreamsIntoChunks(streams []logproto.Stream, maxBytes int) [][]logproto.Stream {
	if len(streams) == 0 {
		return nil
	}

	var chunks [][]logproto.Stream
	currentChunk := make([]logproto.Stream, 0)

	for _, stream := range streams {
		// Create a temporary chunk with the new stream to check its size
		tempChunk := append(currentChunk, stream)
		req := &logproto.PushRequest{
			Streams: tempChunk,
		}
		data, err := proto.Marshal(req)
		if err != nil {
			// If we can't marshal, just add the stream and hope for the best
			currentChunk = append(currentChunk, stream)
			continue
		}

		// If adding this stream would exceed the limit, start a new chunk
		if len(data) > maxBytes && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = make([]logproto.Stream, 0)
		}

		currentChunk = append(currentChunk, stream)
	}

	// Add the last chunk if it's not empty
	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

// sendSamples to the remote storage with backoff for recoverable errors.
func SendStreamsWithBackoff(ctx context.Context, client *prom.Client, streams []logproto.Stream, buf *[]byte) error {
	// Split streams into chunks that fit within maxBytes
	const maxBytes = 255 * 1024 // 255KB, slightly below Loki's 256KB limit
	chunks := splitStreamsIntoChunks(streams, maxBytes)

	for _, chunk := range chunks {
		req, err := buildStreamsPushRequest(chunk, *buf)
		*buf = req
		if err != nil {
			// Failing to build the write request is non-recoverable, since it will
			// only error if marshaling the proto to bytes fails.
			return err
		}

		if err := prom.SendBytesWithBackoff(ctx, client, req); err != nil {
			return fmt.Errorf("sending events: %w", err)
		}
	}

	return nil
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
