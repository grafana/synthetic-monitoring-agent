package backfill

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/golang/snappy"
	logproto "github.com/grafana/loki/pkg/push"
	"github.com/prometheus/prometheus/prompb"
)

var executionIDLabelPattern = regexp.MustCompile(`,?\s*execution_id="[^"]+"`)

// WritePrometheusRemote posts snappy-compressed remote-write payloads to Prometheus.
func WritePrometheusRemote(ctx context.Context, baseURL string, series TimeSeries, client *http.Client) error {
	if len(series) == 0 {
		return nil
	}
	reqBody := &prompb.WriteRequest{Timeseries: series}
	data, err := reqBody.Marshal()
	if err != nil {
		return err
	}
	compressed := snappy.Encode(nil, data)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/write", bytes.NewReader(compressed))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("Content-Encoding", "snappy")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	resp, err := httpClient(client).Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("prometheus remote write HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// LokiPushBatchOptions controls chronological batched pushes for historical backfill.
type LokiPushBatchOptions struct {
	MaxStreamsPerPush int
	MaxEntriesPerPush int
	MaxWindowPerPush  time.Duration
	PauseBetween      time.Duration
}

func (o *LokiPushBatchOptions) withDefaults() LokiPushBatchOptions {
	out := LokiPushBatchOptions{}
	if o != nil {
		out = *o
	}
	if out.MaxStreamsPerPush <= 0 {
		out.MaxStreamsPerPush = 40
	}
	if out.MaxEntriesPerPush <= 0 {
		out.MaxEntriesPerPush = 500
	}
	if out.MaxWindowPerPush <= 0 {
		out.MaxWindowPerPush = 20 * time.Minute
	}
	if out.PauseBetween < 0 {
		out.PauseBetween = 0
	}
	return out
}

// WriteLokiPushBatched pushes log streams in oldest-first batches so Loki ingesters
// are not overwhelmed by a single multi-hour out-of-order stream.
func WriteLokiPushBatched(ctx context.Context, baseURL string, streams Streams, opts *LokiPushBatchOptions, client *http.Client) error {
	if len(streams) == 0 {
		return nil
	}
	batchOpts := opts.withDefaults()
	batches := splitStreamsIntoBatches(streams, batchOpts)
	for i, batch := range batches {
		consolidated := ConsolidateStreams(batch)
		if err := WriteLokiPush(ctx, baseURL, consolidated, client); err != nil {
			return fmt.Errorf("loki push batch %d/%d (%d streams): %w", i+1, len(batches), len(batch), err)
		}
		if batchOpts.PauseBetween > 0 && i < len(batches)-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(batchOpts.PauseBetween):
			}
		}
	}
	return nil
}

func splitStreamsIntoBatches(streams Streams, opts LokiPushBatchOptions) []Streams {
	sorted := append(Streams(nil), streams...)
	sort.Slice(sorted, func(i, j int) bool {
		return streamStartTime(sorted[i]).Before(streamStartTime(sorted[j]))
	})

	var (
		batches        []Streams
		current        Streams
		currentEntries int
		batchStart     time.Time
	)

	flush := func() {
		if len(current) == 0 {
			return
		}
		batches = append(batches, current)
		current = nil
		currentEntries = 0
	}

	for _, stream := range sorted {
		start := streamStartTime(stream)
		entryCount := len(stream.Entries)

		if len(current) > 0 {
			needNewBatch := len(current) >= opts.MaxStreamsPerPush ||
				currentEntries+entryCount > opts.MaxEntriesPerPush ||
				start.Sub(batchStart) > opts.MaxWindowPerPush
			if needNewBatch {
				flush()
			}
		}
		if len(current) == 0 {
			batchStart = start
		}
		current = append(current, stream)
		currentEntries += entryCount
	}
	flush()
	return batches
}

func streamStartTime(stream logproto.Stream) time.Time {
	if len(stream.Entries) == 0 {
		return time.Time{}
	}
	start := stream.Entries[0].Timestamp
	for _, entry := range stream.Entries[1:] {
		if entry.Timestamp.Before(start) {
			start = entry.Timestamp
		}
	}
	return start
}

// WriteLokiPush posts snappy-compressed log streams to Loki's push API.
func WriteLokiPush(ctx context.Context, baseURL string, streams Streams, client *http.Client) error {
	if len(streams) == 0 {
		return nil
	}
	reqBody := logproto.PushRequest{Streams: streams}
	data, err := reqBody.Marshal()
	if err != nil {
		return err
	}
	compressed := snappy.Encode(nil, data)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/loki/api/v1/push", bytes.NewReader(compressed))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("Content-Encoding", "snappy")

	resp, err := httpClient(client).Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("loki push HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ConsolidateStreams merges per-execution streams into one chronologically sorted
// stream per label set. execution_id stays on each entry's structured metadata.
func ConsolidateStreams(streams Streams) Streams {
	if len(streams) <= 1 {
		return streams
	}

	merged := make(map[string]*logproto.Stream)
	for _, stream := range streams {
		labels := stripExecutionIDLabel(stream.Labels)
		existing, ok := merged[labels]
		if !ok {
			copy := logproto.Stream{
				Labels:  labels,
				Entries: append([]logproto.Entry(nil), stream.Entries...),
			}
			merged[labels] = &copy
			continue
		}
		existing.Entries = append(existing.Entries, stream.Entries...)
	}

	out := make(Streams, 0, len(merged))
	for _, stream := range merged {
		entries := stream.Entries
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Timestamp.Before(entries[j].Timestamp)
		})
		stream.Entries = entries
		out = append(out, *stream)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Labels < out[j].Labels
	})
	return out
}

func stripExecutionIDLabel(labels string) string {
	labels = executionIDLabelPattern.ReplaceAllString(labels, "")
	return normalizeLabelSet(labels)
}

func normalizeLabelSet(labels string) string {
	labels = strings.TrimSpace(labels)
	labels = strings.ReplaceAll(labels, "{,", "{")
	labels = strings.ReplaceAll(labels, ",}", "}")
	return labels
}

func httpClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return http.DefaultClient
}
