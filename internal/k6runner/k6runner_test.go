package k6runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	r1 := New(RunnerOpts{Uri: "k6"})
	require.IsType(t, Local{}, r1)
	require.Equal(t, "", r1.(Local).blacklistedIP)

	r2 := New(RunnerOpts{Uri: "/usr/bin/k6", BlacklistedIP: "192.168.4.0/24"})
	require.IsType(t, Local{}, r2)
	require.Equal(t, "192.168.4.0/24", r2.(Local).blacklistedIP)
	// Ensure WithLogger preserves config.
	zl := zerolog.New(io.Discard)
	r2 = r2.WithLogger(&zl)
	require.Equal(t, "192.168.4.0/24", r2.(Local).blacklistedIP)

	r3 := New(RunnerOpts{Uri: "http://localhost:6565"})
	require.IsType(t, &HttpRunner{}, r3)
	r4 := New(RunnerOpts{Uri: "https://localhost:6565"})
	require.IsType(t, &HttpRunner{}, r4)
}

func TestNewScript(t *testing.T) {
	t.Parallel()

	runner := New(RunnerOpts{Uri: "k6"})
	script := Script{
		Script: []byte("test"),
		Settings: Settings{
			Timeout: 1000,
		},
	}

	processor, err := NewProcessor(script, runner)
	require.NoError(t, err)
	require.NotNil(t, processor)
	require.Equal(t, script, processor.script)
	require.Equal(t, runner, processor.runner)
}

func TestScriptRun(t *testing.T) {
	t.Parallel()

	runner := testRunner{
		metrics: testhelper.MustReadFile(t, "testdata/test.out"),
		logs:    testhelper.MustReadFile(t, "testdata/test.log"),
	}

	processor, err := NewProcessor(Script{
		Script: testhelper.MustReadFile(t, "testdata/test.js"),
		Settings: Settings{
			Timeout: 1000,
		},
	}, &runner)
	require.NoError(t, err)
	require.NotNil(t, processor)

	var (
		registry = prometheus.NewRegistry()
		logger   testLogger
		buf      bytes.Buffer
		zlogger  = zerolog.New(&buf)
	)

	ctx, cancel := testhelper.Context(context.Background(), t)
	t.Cleanup(cancel)

	// We already know tha parsing the metrics and the logs is working, so
	// we are only interested in verifying that the script runs without
	// errors.
	success, duration, err := processor.Run(ctx, registry, &logger, zlogger, SecretStore{})
	require.NoError(t, err)
	require.True(t, success)
	require.Equal(t, 500*time.Millisecond, duration)
}

func TestCheckInfoFromSM(t *testing.T) {
	t.Parallel()

	check := model.Check{
		RegionId: 4,
		Check: sm.Check{
			Id:       69,
			TenantId: 1234,
			Created:  1234.5,
			Modified: 12345.6,
			Settings: sm.CheckSettings{
				Browser: &sm.BrowserSettings{}, // Make it non-nil so type is Browser.
			},
		},
	}

	ci := CheckInfoFromSM(check)

	require.Equal(t, sm.CheckTypeBrowser.String(), ci.Type)
	require.Equal(t, map[string]any{
		"id":       check.Id,
		"tenantID": check.TenantId,
		"regionID": check.RegionId,
		"created":  check.Created,
		"modified": check.Modified,
	}, ci.Metadata)
}

type testRunner struct {
	metrics []byte
	logs    []byte
}

var _ Runner = &testRunner{}

func (r *testRunner) Run(ctx context.Context, script Script, secretStore SecretStore) (*RunResponse, error) {
	return &RunResponse{
		Metrics: r.metrics,
		Logs:    r.logs,
	}, nil
}

func (r *testRunner) WithLogger(logger *zerolog.Logger) Runner {
	return r
}

func TestTextToRegistry(t *testing.T) {
	data := testhelper.MustReadFile(t, "testdata/test.out")

	expectedMetrics := map[string]struct{}{}

	promDecoder := expfmt.NewDecoder(bytes.NewBuffer(data), expfmt.NewFormat(expfmt.TypeTextPlain))
DEC_LOOP:
	for {
		var mf dto.MetricFamily
		switch err := promDecoder.Decode(&mf); err {
		case nil:
			for _, m := range mf.GetMetric() {
				expectedMetrics[buildId(mf.GetName(), m)] = struct{}{}
			}
		case io.EOF:
			break DEC_LOOP
		default:
			t.Fatal(err)
		}
	}

	var sampleCollector sampleCollector

	buf := bytes.Buffer{}
	logger := zerolog.New(&buf)

	err := extractMetricSamples(data, logger, sampleCollector.process)
	require.NoError(t, err)

	registry := prometheus.NewRegistry()
	err = registry.Register(&sampleCollector.collector)
	require.NoError(t, err)

	mfs, err := registry.Gather()
	require.NoError(t, err)

	actualMetrics := map[string]struct{}{}
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			actualMetrics[buildId(mf.GetName(), m)] = struct{}{}
		}
	}

	// This is some minimal validation that all the metrics are parsed and
	// added to the registry.
	require.Equal(t, expectedMetrics, actualMetrics)
}

func buildId(name string, m *dto.Metric) string {
	labels := m.GetLabel()
	sort.Slice(labels, func(i, j int) bool {
		switch rel := strings.Compare(labels[i].GetName(), labels[j].GetName()); rel {
		case 0:
			return strings.Compare(labels[i].GetValue(), labels[j].GetValue()) < 0
		default:
			return rel < 0
		}
	})

	var s strings.Builder
	s.WriteString(name)
	for _, l := range labels {
		s.WriteString(",")
		s.WriteString(l.GetName())
		s.WriteString("=")
		s.WriteString(l.GetValue())
	}

	return s.String()
}

func TestK6LogsToLogger(t *testing.T) {
	data := testhelper.MustReadFile(t, "testdata/test.log")

	var logger testLogger

	err := k6LogsToLogger(data, &logger)
	require.NoError(t, err)
}

type testLogger struct{}

var _ logger.Logger = &testLogger{}

func (l *testLogger) Log(keyvals ...any) error {
	if len(keyvals) == 0 {
		return errors.New("empty log message")
	}
	return nil
}

type recordingLogger struct {
	buf *bytes.Buffer
}

var _ logger.Logger = &recordingLogger{}

func (l recordingLogger) Log(keyvals ...any) error {
	if len(keyvals) == 0 {
		return errors.New("empty log message")
	}

	if len(keyvals)%2 != 0 {
		return errors.New("not the same number of keys and vals")
	}

	line := make([]string, 0, len(keyvals)/2)
	for i := 0; i < len(keyvals); i += 2 {
		key := keyvals[i]
		val := keyvals[i+1]

		line = append(line, fmt.Sprintf("%s=%q", key, val))
	}

	_, err := fmt.Fprintln(l.buf, strings.Join(line, " "))
	return err
}
