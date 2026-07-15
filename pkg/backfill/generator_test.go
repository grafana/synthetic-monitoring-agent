package backfill_test

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/pkg/backfill"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	prommodel "github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func testCheck() model.Check {
	return model.Check{
		Check: sm.Check{
			Id:        42,
			TenantId:  1,
			Target:    "https://shop.example",
			Job:       "backfill-http",
			Frequency: 60_000,
			Timeout:   5000,
			Modified:  1700000000,
			Settings: sm.CheckSettings{
				Http: &sm.HttpSettings{
					IpVersion: sm.IpVersion_V4,
				},
			},
		},
		RegionId: 0,
	}
}

func testProbe() sm.Probe {
	return sm.Probe{
		Id:        1,
		Name:      "probe-1",
		Region:    "dev",
		Latitude:  51.5,
		Longitude: -0.12,
	}
}

func testSample(at time.Time) backfill.Sample {
	return backfill.Sample{
		At:                at,
		Success:           true,
		StatusCode:        200,
		DurationSeconds:   0.2,
		DNSLookupSeconds:  0.000004333,
		ResolveSeconds:    0.000004333,
		ConnectSeconds:    0.001548209,
		ProcessingSeconds: 0.17,
		TransferSeconds:   0.00133,
	}
}

func metricNames(ts backfill.TimeSeries) []string {
	names := make(map[string]struct{})
	for _, series := range ts {
		for _, label := range series.Labels {
			if label.Name == prommodel.MetricNameLabel {
				names[label.Value] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func TestGeneratorCollectGoldenParity(t *testing.T) {
	ctx := context.Background()
	gen, err := backfill.NewGenerator(ctx, testCheck(), testProbe())
	require.NoError(t, err)

	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	ts, streams, err := gen.Collect(ctx, at, testSample(at))
	require.NoError(t, err)
	require.NotEmpty(t, ts)
	require.NotEmpty(t, streams)

	names := metricNames(ts)
	require.Contains(t, names, "probe_http_status_code")
	require.Contains(t, names, "probe_success")
	require.Contains(t, names, "sm_check_info")
	require.Contains(t, names, "probe_all_duration_seconds_sum")
	require.Contains(t, names, "probe_all_success_sum")

	var executionID string
	for _, stream := range streams {
		require.NotEmpty(t, stream.Labels)
		require.NotEmpty(t, stream.Entries)
		for _, entry := range stream.Entries {
			require.NotEmpty(t, entry.StructuredMetadata)
			if executionID == "" {
				executionID = entry.StructuredMetadata[0].Value
			}
			require.Equal(t, executionID, entry.StructuredMetadata[0].Value)
		}
	}
	require.NotEmpty(t, executionID)
}

func seriesSignatures(ts backfill.TimeSeries) []string {
	sigs := make([]string, 0, len(ts))
	for _, series := range ts {
		labels := make([]string, 0, len(series.Labels))
		for _, label := range series.Labels {
			labels = append(labels, label.Name+"="+label.Value)
		}
		sort.Strings(labels)
		value := ""
		tsMs := int64(0)
		if len(series.Samples) > 0 {
			value = formatFloat(series.Samples[0].Value)
			tsMs = series.Samples[0].Timestamp
		}
		sigs = append(sigs, strings.Join(labels, ",")+"|"+value+"|"+strconv.FormatInt(tsMs, 10))
	}
	sort.Strings(sigs)
	return sigs
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func TestGeneratorCollectMetricsDeterministicWithUniqueExecutionIDs(t *testing.T) {
	ctx := context.Background()
	check := testCheck()
	probe := testProbe()
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sample := testSample(at)

	run := func() ([]string, string) {
		gen, err := backfill.NewGenerator(ctx, check, probe)
		require.NoError(t, err)
		ts, streams, err := gen.Collect(ctx, at, sample)
		require.NoError(t, err)
		require.NotEmpty(t, streams)
		executionID := ""
		for _, stream := range streams {
			for _, entry := range stream.Entries {
				require.NotEmpty(t, entry.StructuredMetadata)
				if executionID == "" {
					executionID = entry.StructuredMetadata[0].Value
				}
				require.Equal(t, executionID, entry.StructuredMetadata[0].Value)
			}
		}
		require.NotEmpty(t, executionID)
		return seriesSignatures(ts), executionID
	}

	first, firstExecutionID := run()
	second, secondExecutionID := run()
	require.Equal(t, first, second)
	require.NotEqual(t, firstExecutionID, secondExecutionID)
}

func TestEndLogUsesCollectTimestamp(t *testing.T) {
	ctx := context.Background()
	gen, err := backfill.NewGenerator(ctx, testCheck(), testProbe())
	require.NoError(t, err)

	at := time.Date(2026, 7, 7, 14, 47, 0, 0, time.UTC)
	sample := testSample(at)
	_, streams, err := gen.Collect(ctx, at, sample)
	require.NoError(t, err)
	require.NotEmpty(t, streams)

	var endEntry time.Time
	for _, stream := range streams {
		for _, entry := range stream.Entries {
			if strings.Contains(entry.Line, "duration_seconds=") {
				endEntry = entry.Timestamp
			}
		}
	}
	require.False(t, endEntry.IsZero())
	require.Equal(t, at.Add(200*time.Millisecond).UTC(), endEntry.UTC())
}

func TestGeneratorResponseTimingsLogHasBBETimestampLabels(t *testing.T) {
	ctx := context.Background()
	gen, err := backfill.NewGenerator(ctx, testCheck(), testProbe())
	require.NoError(t, err)

	at := time.Date(2026, 7, 8, 8, 1, 37, 0, time.UTC)
	_, streams, err := gen.Collect(ctx, at, testSample(at))
	require.NoError(t, err)

	var timingsLine string
	for _, stream := range streams {
		for _, entry := range stream.Entries {
			if strings.Contains(entry.Line, `msg="Response timings for roundtrip"`) {
				timingsLine = entry.Line
				break
			}
		}
	}
	require.NotEmpty(t, timingsLine)
	for _, key := range []string{"start=", "dnsDone=", "connectDone=", "gotConn=", "responseStart=", "end=", "roundtrip="} {
		require.Contains(t, timingsLine, key)
	}
	require.NotContains(t, timingsLine, "processing=")
}

func TestSampleNormalize(t *testing.T) {
	sample := backfill.Sample{
		Success:         true,
		DurationSeconds: 0.25,
		ResolveSeconds:  0.001,
		ConnectSeconds:  0.002,
		TransferSeconds: 0.003,
	}
	sample.Normalize()
	require.Equal(t, 200, sample.StatusCode)
	require.InDelta(t, 0.244, sample.ProcessingSeconds, 0.0001)
}
