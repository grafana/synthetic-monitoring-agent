package collector_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-logfmt/logfmt"
	"github.com/google/uuid"
	"github.com/grafana/synthetic-monitoring-agent/pkg/collector"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

type suppliedProbe struct {
	eventTime time.Time
	success   bool
}

func (suppliedProbe) Name() string { return "supplied" }

func (p suppliedProbe) Probe(_ context.Context, _ string, registry *prometheus.Registry, logger collector.Logger) (bool, float64) {
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "probe_supplied_value"})
	registry.MustRegister(gauge)
	gauge.Set(7)

	_ = logger.Log(
		"level", "INFO",
		"msg", "supplied probe detail",
		"time", p.eventTime.Format(time.RFC3339Nano),
	)

	return p.success, 0.25
}

type untimedProbe struct{}

func (untimedProbe) Name() string { return "untimed" }

func (untimedProbe) Probe(_ context.Context, _ string, _ *prometheus.Registry, logger collector.Logger) (bool, float64) {
	_ = logger.Log("level", "INFO", "msg", "untimed-probe-detail")

	return true, 0.25
}

func TestCollectUsesLogicalEventTimeAndAgentExecutionMetadata(t *testing.T) {
	eventTime := time.Date(2020, 3, 1, 12, 0, 0, 0, time.UTC)
	c, err := collector.New(context.Background(), testCheck(), testProbe(), suppliedProbe{
		eventTime: eventTime.Add(50 * time.Millisecond),
		success:   true,
	})
	require.NoError(t, err)

	series, streams, err := c.Collect(context.Background(), eventTime)
	require.NoError(t, err)
	require.NotEmpty(t, series)
	require.Len(t, streams, 1)

	foundMetric := false

	for _, ts := range series {
		for _, label := range ts.Labels {
			if label.Name == "__name__" && label.Value == "probe_supplied_value" {
				foundMetric = true

				require.Len(t, ts.Samples, 1)
				require.Equal(t, eventTime.UnixMilli(), ts.Samples[0].Timestamp)
			}
		}
	}

	require.True(t, foundMetric)

	var (
		executionID string
		walltime    float64
	)

	for _, entry := range streams[0].Entries {
		require.Len(t, entry.StructuredMetadata, 1)
		metadata := entry.StructuredMetadata[0]
		require.Equal(t, "execution_id", metadata.Name)

		if executionID == "" {
			executionID = metadata.Value
		}

		require.Equal(t, executionID, metadata.Value)

		if strings.Contains(entry.Line, "msg=\"Check succeeded\"") {
			walltime = logfmtFloat(t, entry.Line, "walltime_seconds")
			require.Equal(t, eventTime.Add(250*time.Millisecond), entry.Timestamp)
		}

		if strings.Contains(entry.Line, "msg=\"Beginning check\"") {
			require.Equal(t, eventTime, entry.Timestamp, "beginning-check log must carry the logical event start time")
		}
	}

	require.NoError(t, uuid.Validate(executionID))
	require.Positive(t, walltime)
	require.NotEqual(t, 0.25, walltime, "walltime must measure execution, not logical event duration")
}

func TestCollectReturnsFailedProbeTelemetry(t *testing.T) {
	eventTime := time.Date(2020, 3, 1, 12, 0, 0, 0, time.UTC)
	c, err := collector.New(context.Background(), testCheck(), testProbe(), suppliedProbe{
		eventTime: eventTime,
		success:   false,
	})
	require.NoError(t, err)

	series, streams, err := c.Collect(context.Background(), eventTime)
	require.Error(t, err)
	require.NotEmpty(t, series)
	require.NotEmpty(t, streams)
}

func TestCollectUsesLogicalEventTimeForUntimedProbeLogs(t *testing.T) {
	eventTime := time.Date(2020, 3, 1, 12, 0, 0, 0, time.UTC)
	c, err := collector.New(context.Background(), testCheck(), testProbe(), untimedProbe{})
	require.NoError(t, err)

	_, streams, err := c.Collect(context.Background(), eventTime)
	require.NoError(t, err)
	require.Len(t, streams, 1)

	foundUntimedLog := false

	for _, entry := range streams[0].Entries {
		if strings.Contains(entry.Line, "msg=untimed-probe-detail") {
			foundUntimedLog = true

			require.Equal(t, eventTime, entry.Timestamp, "probe logs without an explicit time must use the logical event time")
		}
	}

	require.True(t, foundUntimedLog, "expected the injected probe log in collected streams")
}

func logfmtFloat(t *testing.T, line, wanted string) float64 {
	t.Helper()

	decoder := logfmt.NewDecoder(strings.NewReader(line))
	for decoder.ScanRecord() {
		for decoder.ScanKeyval() {
			if string(decoder.Key()) == wanted {
				value, err := strconv.ParseFloat(string(decoder.Value()), 64)
				require.NoError(t, err)

				return value
			}
		}
	}

	require.NoError(t, decoder.Err())
	t.Fatalf("log field %q not found in %q", wanted, line)

	return 0
}

func testCheck() sm.Check {
	return sm.Check{
		Id:        42,
		TenantId:  1,
		Target:    "https://example.com",
		Job:       "collector-test",
		Frequency: 60_000,
		Timeout:   5_000,
		Settings: sm.CheckSettings{
			Http: &sm.HttpSettings{IpVersion: sm.IpVersion_V4},
		},
	}
}

func testProbe() sm.Probe {
	return sm.Probe{Name: "test-probe", Region: "test-region"}
}
