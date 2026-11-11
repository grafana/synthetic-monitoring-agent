package adhoc

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// errorProber is a prober that can tell a check that failed apart from a
// check that could not be run, like the ones that run k6 scripts.
type errorProber struct {
	// run is called by ProbeErr.
	run func(ctx context.Context, l logger.Logger) (bool, float64, error)
}

var _ prober.ErrorProber = errorProber{}

func (errorProber) Name() string { return "errorprober" }

func (p errorProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger, executionID string) (bool, float64) {
	success, duration, _ := p.ProbeErr(ctx, target, registry, l, executionID)

	return success, duration
}

func (p errorProber) ProbeErr(ctx context.Context, _ string, _ *prometheus.Registry, l logger.Logger, _ string) (bool, float64, error) {
	return p.run(ctx, l)
}

// runWith runs a single execution with the given prober through the
// handler, and returns the result reported to the API and the Loki line.
func runWith(t *testing.T, p proberLike, timeout time.Duration, omitReason bool) (*sm.AdHocResult, string) {
	t.Helper()

	srv := newTransportServer(transportRequest("classification"))

	publishCh := startTransportHandler(t, srv,
		func(_ context.Context, req *sm.AdHocRequest) (*runner, error) {
			return &runner{
				logger:     zerolog.New(io.Discard),
				prober:     p,
				id:         req.AdHocCheck.Id,
				target:     req.AdHocCheck.Target,
				probe:      "transportProbe",
				timeout:    timeout,
				omitReason: omitReason,
			}, nil
		})

	result := srv.waitReported(t)
	payload := waitPublished(t, publishCh)

	return result, payload.Streams()[0].Entries[0].Line
}

func logsText(result *sm.AdHocResult) string {
	var b strings.Builder

	for _, l := range result.Logs {
		b.Write(l)
		b.WriteString("\n")
	}

	return b.String()
}

// A check that could not be run is not a check that failed.
func TestClassificationCouldNotRun(t *testing.T) {
	const internal = "internal-runner.example:8080"

	result, lokiLine := runWith(t,
		errorProber{run: func(context.Context, logger.Logger) (bool, float64, error) {
			return false, 0, errors.New(`Post "http://` + internal + `/run": connection refused`)
		}},
		10*time.Second, false)

	require.Equal(t, sm.AdHocResult_COULD_NOT_START, result.Status)
	require.Equal(t, couldNotRunMessage, result.Error)

	// The cause is for the agent's own log: it can name the infrastructure.
	require.NotContains(t, result.Error, internal)
	require.NotContains(t, logsText(result), internal)
	require.NotContains(t, lokiLine, internal)

	// But the caller can see in the logs where the failure happened.
	require.Contains(t, logsText(result), couldNotRunMessage)
}

// If the check ran out of time, that is what happened, whatever else the
// prober reports on the way out.
func TestClassificationTimeoutWins(t *testing.T) {
	result, _ := runWith(t,
		errorProber{run: func(ctx context.Context, _ logger.Logger) (bool, float64, error) {
			<-ctx.Done()

			return false, 0, ctx.Err()
		}},
		50*time.Millisecond, false)

	require.Equal(t, sm.AdHocResult_TIMEOUT, result.Status)
}

// A check that ran and did not pass is still a failure.
func TestClassificationCheckFailed(t *testing.T) {
	result, _ := runWith(t,
		errorProber{run: func(_ context.Context, l logger.Logger) (bool, float64, error) {
			_ = l.Log("level", "error", "msg", "assertion failed")

			return false, 0.5, nil
		}},
		10*time.Second, false)

	require.Equal(t, sm.AdHocResult_FAILED, result.Status)
	require.Equal(t, "assertion failed", result.Error, "the reason is kept for checks that do not run scripts")
}

// What a script printed is not something the tenant can read from the
// check configuration, so it is not turned into the reason.
func TestClassificationOmitReason(t *testing.T) {
	result, _ := runWith(t,
		errorProber{run: func(_ context.Context, l logger.Logger) (bool, float64, error) {
			_ = l.Log("level", "error", "msg", "script said hunter2")

			return false, 0.5, nil
		}},
		10*time.Second, true)

	require.Equal(t, sm.AdHocResult_FAILED, result.Status)
	require.Empty(t, result.Error)
	require.Contains(t, logsText(result), "script said", "the logs still carry what happened")
}

func TestUsesK6(t *testing.T) {
	for name, tc := range map[string]struct {
		check sm.CheckType
		want  bool
	}{
		"scripted":   {sm.CheckTypeScripted, true},
		"browser":    {sm.CheckTypeBrowser, true},
		"multihttp":  {sm.CheckTypeMultiHttp, true},
		"http":       {sm.CheckTypeHttp, false},
		"ping":       {sm.CheckTypePing, false},
		"dns":        {sm.CheckTypeDns, false},
		"tcp":        {sm.CheckTypeTcp, false},
		"traceroute": {sm.CheckTypeTraceroute, false},
		"grpc":       {sm.CheckTypeGrpc, false},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, usesK6(tc.check))
		})
	}
}
