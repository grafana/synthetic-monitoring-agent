package checks

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/scraper"
	"github.com/grafana/synthetic-monitoring-agent/internal/telemetry"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

func TestNewUpdater(t *testing.T) {
	testFeatureCollection := feature.NewCollection()
	require.NotNil(t, testFeatureCollection)
	require.NoError(t, testFeatureCollection.Set("foo"))
	require.NoError(t, testFeatureCollection.Set("bar"))

	testcases := map[string]struct {
		opts UpdaterOptions
	}{
		"trivial": {
			opts: UpdaterOptions{
				Conn:           new(grpc.ClientConn),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				Publisher:      channelPublisher(make(chan pusher.Payload)),
				TenantCh:       make(chan<- sm.Tenant),
				Logger:         testhelper.Logger(t),
				Features:       testFeatureCollection,
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			u, err := NewUpdater(tc.opts)
			require.NoError(t, err)
			require.NotNil(t, u)
			require.Equal(t, tc.opts.Publisher, u.publisher)
			require.Equal(t, tc.opts.TenantCh, u.tenantCh)
			require.Equal(t, tc.opts.Features, u.features)
			require.Equal(t, tc.opts.Logger, u.logger)
			require.Equal(t, tc.opts.Conn, u.api.conn)
			require.NotNil(t, u.scrapers)
			require.NotNil(t, u.metrics.changesCounter)
			require.NotNil(t, u.metrics.changeErrorsCounter)
			require.NotNil(t, u.metrics.runningScrapers)
			require.NotNil(t, u.metrics.scrapesCounter)
			require.NotNil(t, u.metrics.scrapeErrorCounter)
			require.NotNil(t, u.metrics.probeInfo)
			require.False(t, u.supportsProtocolSecrets, "default value should be false")
		})
	}
}

func TestNewUpdaterSupportsProtocolSecrets(t *testing.T) {
	testFeatureCollection := feature.NewCollection()
	require.NotNil(t, testFeatureCollection)

	opts := UpdaterOptions{
		Conn:                    new(grpc.ClientConn),
		PromRegisterer:          prometheus.NewPedanticRegistry(),
		Publisher:               channelPublisher(make(chan pusher.Payload)),
		TenantCh:                make(chan<- sm.Tenant),
		Logger:                  testhelper.Logger(t),
		Features:                testFeatureCollection,
		SupportsProtocolSecrets: true,
	}

	u, err := NewUpdater(opts)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.True(t, u.supportsProtocolSecrets, "should be set to true")
}

func TestInstallSignalHandler(t *testing.T) {
	testcases := map[string]func(t *testing.T){
		"signal": func(t *testing.T) {
			// verify that the signal context is done after
			// receiving the signal, and that the signal is
			// correctly reported as having fired.
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			sigCtx, signalFired := installSignalHandler(ctx)
			require.NotNil(t, sigCtx)
			require.NotNil(t, signalFired)
			require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGUSR1))

			select {
			case <-ctx.Done():
				t.Fatal("context timeout expired")
			case <-sigCtx.Done():
				require.Equal(t, int32(1), atomic.LoadInt32(signalFired))
			}
		},

		"no signal": func(t *testing.T) {
			// verify that the signal context is done after
			// the parrent context is done, and that the
			// signal is correctly reported as not having
			// fired.
			ctx, cancel := context.WithCancel(context.Background())
			sigCtx, signalFired := installSignalHandler(ctx)
			require.NotNil(t, sigCtx)
			require.NotNil(t, signalFired)

			cancel()

			timeout := 100 * time.Millisecond

			timer := time.NewTimer(timeout)
			defer timer.Stop()

			select {
			case <-timer.C:
				t.Fatalf("signal context not cancelled after %s", timeout)
			case <-sigCtx.Done():
				require.Equal(t, int32(0), atomic.LoadInt32(signalFired))
			}
		},
	}

	for name, f := range testcases {
		t.Run(name, f)
	}
}

func TestSleepCtx(t *testing.T) {
	var (
		veryShort = 1 * time.Microsecond
		long      = 10 * time.Second
	)

	// make sure errors are reported correctly

	ctx := context.Background()
	err := sleepCtx(ctx, veryShort)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = sleepCtx(ctx, long)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)

	ctx, cancel = context.WithTimeout(context.Background(), veryShort)
	err = sleepCtx(ctx, long)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	cancel()

	ctx, cancel = context.WithTimeout(context.Background(), long)
	cancel()

	err = sleepCtx(ctx, long)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// TestHandleCheckOp is testing internal functions that run as part of
// updater.Run. Since these functions operate on scraper instances, a
// test scraper is used, which in turn creates a test probe. The goal of
// this is to decouple the testing of these functions from the testing
// of the prober themselves.
func TestHandleCheckOp(t *testing.T) {
	synctest.Test(t, testHandleCheckOpImpl)
}

// testHandleCheckOpImpl is the actual implementation of the
// TestHandleCheckOp test above to avoid excessive indentation.
func testHandleCheckOpImpl(t *testing.T) {
	publishCh := make(chan pusher.Payload, 100)

	u, err := NewUpdater(
		UpdaterOptions{
			Conn:           new(grpc.ClientConn),
			PromRegisterer: prometheus.NewPedanticRegistry(),
			Publisher:      channelPublisher(publishCh),
			TenantCh:       make(chan<- sm.Tenant),
			Logger:         testhelper.Logger(t),
			ScraperFactory: testScraperFactory,
		},
	)

	require.NotNil(t, u)
	require.NoError(t, err)

	u.probe = &sm.Probe{
		Id:   100,
		Name: "test-probe",
	}

	// Since this is meant to run inside a test bubble, t.Deadline cannot be used.
	// The timeout here doesn't really matter because this is running with faketime.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	var check model.Check

	err = check.FromSM(sm.Check{
		Id:        5000,
		TenantId:  1,
		Frequency: 1000,
		Timeout:   1000,
		Target:    "127.0.0.1",
		Job:       "", // not setting value to make check invalid
		Probes:    []int64{1},
		Settings: sm.CheckSettings{
			Ping: &sm.PingSettings{},
		},
		Created:  0,
		Modified: 0,
	})
	require.NoError(t, err)

	scraperExists := func() bool {
		_, found := u.scrapers.get(check.GlobalID())

		return found
	}

	// this should fail, check is invalid
	err = u.handleCheckAdd(ctx, check)
	require.Error(t, err)
	// This doesn't work because the counter hasn't been set
	// (because of the error):
	// require.Equal(t, 0.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.False(t, scraperExists())
	requireRegistryConsistent(t, u.scrapers)

	// fix check
	check.Job = "test-job"
	check.Modified++

	err = u.handleCheckAdd(ctx, check)
	require.NoError(t, err)
	require.Equal(t, 1.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.True(t, scraperExists())
	requireRegistryConsistent(t, u.scrapers)

	check.Modified++

	// try to add again, this should fail, even if modified changed
	err = u.handleCheckAdd(ctx, check)
	require.Error(t, err)
	require.Equal(t, 1.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.True(t, scraperExists())
	requireRegistryConsistent(t, u.scrapers)

	check.Modified++

	// update the existing check
	err = u.handleCheckUpdate(ctx, check)
	require.NoError(t, err)
	require.Equal(t, 1.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.True(t, scraperExists())
	requireRegistryConsistent(t, u.scrapers)

	err = u.handleCheckDelete(ctx, check)
	require.NoError(t, err)
	require.Equal(t, 0.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.False(t, scraperExists())
	requireRegistryConsistent(t, u.scrapers)

	// try to delete again
	err = u.handleCheckDelete(ctx, check)
	require.Error(t, err)
	require.Equal(t, 0.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.False(t, scraperExists())
	requireRegistryConsistent(t, u.scrapers)

	// updating a non-existing check becomes an add
	err = u.handleCheckUpdate(ctx, check)
	require.NoError(t, err)
	require.Equal(t, 1.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.True(t, scraperExists())
	requireRegistryConsistent(t, u.scrapers)

	// clean up
	err = u.handleCheckDelete(ctx, check)
	require.NoError(t, err)
	require.Equal(t, 0.0, testutil.ToFloat64(u.metrics.runningScrapers))
	require.False(t, scraperExists())
	requireRegistryConsistent(t, u.scrapers)

	// Wait for all scraper goroutines to fully exit before test completes.
	synctest.Wait()
}

func TestHandleTenantUpdate(t *testing.T) {
	synctest.Test(t, testHandleTenantUpdateImpl)
}

func testHandleTenantUpdateImpl(t *testing.T) {
	u, err := NewUpdater(
		UpdaterOptions{
			Conn:           new(grpc.ClientConn),
			PromRegisterer: prometheus.NewPedanticRegistry(),
			Publisher:      channelPublisher(make(chan pusher.Payload, 100)),
			TenantCh:       make(chan sm.Tenant, 10),
			Logger:         testhelper.Logger(t),
			ScraperFactory: testScraperFactory,
		},
	)
	require.NoError(t, err)

	u.probe = &sm.Probe{Id: 100, Name: "test-probe"}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	readLastLabelMode := func(id model.GlobalID) (sm.LabelMode, bool) {
		u.scrapers.mu.Lock()
		defer u.scrapers.mu.Unlock()

		mode, ok := u.scrapers.labelModes[id]

		return mode, ok
	}

	// A tenant with zero running scrapers (the registry is still empty at
	// this point in the test) must not panic and must not add anything to
	// the registry, but it must still record the tenant's mode so a later
	// update for the same tenant can tell a no-op change from a real one.
	require.NotPanics(t, func() {
		u.handleTenantUpdate(ctx, sm.Tenant{Id: 7, LabelMode: sm.LabelMode_LABEL_MODE_UNPREFIXED})
	})
	requireRegistryConsistent(t, u.scrapers)

	require.Zero(t, registryLen(u.scrapers), "handleTenantUpdate must not add scrapers for a tenant with none running")

	recordedMode, seen := readLastLabelMode(model.GlobalID(7))
	require.True(t, seen, "labelModes should record the tenant even though it had zero running scrapers")
	require.Equal(t, sm.LabelMode_LABEL_MODE_UNPREFIXED, recordedMode)

	// record-always means a follow-up identical update is a no-op
	require.NotPanics(t, func() {
		u.handleTenantUpdate(ctx, sm.Tenant{Id: 7, LabelMode: sm.LabelMode_LABEL_MODE_UNPREFIXED})
	})
	require.Zero(t, registryLen(u.scrapers))
	requireRegistryConsistent(t, u.scrapers)

	newCheck := func(id int64) model.Check {
		var check model.Check

		err := check.FromSM(sm.Check{
			Id:        id,
			TenantId:  42,
			Frequency: 1000,
			Timeout:   1000,
			Target:    "127.0.0.1",
			Job:       "test-job",
			Probes:    []int64{1},
			Settings:  sm.CheckSettings{Ping: &sm.PingSettings{}},
		})
		require.NoError(t, err)

		return check
	}

	checkA := newCheck(5001)
	checkB := newCheck(5002)

	require.NoError(t, u.handleCheckAdd(ctx, checkA))
	require.NoError(t, u.handleCheckAdd(ctx, checkB))
	requireRegistryConsistent(t, u.scrapers)

	scraperPointer := func(id model.GlobalID) *scraper.Scraper {
		s, _ := u.scrapers.get(id)
		return s
	}

	beforeA := scraperPointer(checkA.GlobalID())
	beforeB := scraperPointer(checkB.GlobalID())

	require.NotNil(t, beforeA)
	require.NotNil(t, beforeB)

	// First sighting of tenant 42: restart happens even though the mode
	// (PREFIXED, the zero value) matches what an unseen tenant would
	// default to if the zero value were mistaken for "no change".
	u.handleTenantUpdate(ctx, sm.Tenant{Id: 42, LabelMode: sm.LabelMode_LABEL_MODE_PREFIXED})
	requireRegistryConsistent(t, u.scrapers)

	afterFirstA := scraperPointer(checkA.GlobalID())
	afterFirstB := scraperPointer(checkB.GlobalID())

	require.NotSame(t, beforeA, afterFirstA, "checkA's scraper should have been replaced on first sighting of its tenant")
	require.NotSame(t, beforeB, afterFirstB, "checkB's scraper should have been replaced on first sighting of its tenant")

	// Same mode again: no restart.
	u.handleTenantUpdate(ctx, sm.Tenant{Id: 42, LabelMode: sm.LabelMode_LABEL_MODE_PREFIXED})
	requireRegistryConsistent(t, u.scrapers)

	afterRepeatA := scraperPointer(checkA.GlobalID())
	afterRepeatB := scraperPointer(checkB.GlobalID())

	require.Same(t, afterFirstA, afterRepeatA, "no label mode change should not restart checkA's scraper")
	require.Same(t, afterFirstB, afterRepeatB, "no label mode change should not restart checkB's scraper")

	// Different mode: restart again, but only for this tenant. Add an
	// unrelated check for a different tenant first and confirm it is left
	// alone by the restart.
	otherCheck := newCheck(5003)
	otherCheck.TenantId = 99
	require.NoError(t, u.handleCheckAdd(ctx, otherCheck))
	requireRegistryConsistent(t, u.scrapers)

	beforeOther := scraperPointer(otherCheck.GlobalID())
	require.NotNil(t, beforeOther)

	u.handleTenantUpdate(ctx, sm.Tenant{Id: 42, LabelMode: sm.LabelMode_LABEL_MODE_DUAL_WRITE})
	requireRegistryConsistent(t, u.scrapers)

	afterChangedA := scraperPointer(checkA.GlobalID())
	afterChangedB := scraperPointer(checkB.GlobalID())
	afterOther := scraperPointer(otherCheck.GlobalID())

	require.NotSame(t, afterRepeatA, afterChangedA, "checkA's scraper should have been replaced when its tenant's label mode changed")
	require.NotSame(t, afterRepeatB, afterChangedB, "checkB's scraper should have been replaced when its tenant's label mode changed")
	require.Same(t, beforeOther, afterOther, "a different tenant's scraper must not be restarted")

	// Clean up: stop every scraper this test started before it exits.
	require.NoError(t, u.handleCheckDelete(ctx, checkA))
	require.NoError(t, u.handleCheckDelete(ctx, checkB))
	require.NoError(t, u.handleCheckDelete(ctx, otherCheck))
	requireRegistryConsistent(t, u.scrapers)
	synctest.Wait()
}

// TestHandleTenantUpdateRestartFailure exercises handleTenantUpdate's error
// branch: one check's scraper fails to restart (its factory returns an
// error) while another check in the same tenant restarts successfully. It
// asserts the failing check keeps its old scraper (restartCheck builds the
// replacement before stopping anything), that the failure doesn't abort
// the loop over the rest of the tenant's checks, and that the failure is
// surfaced on changeErrorsCounter (not just logged) per the "update" label
// already used by every other restart-style failure path in this file.
func TestHandleTenantUpdateRestartFailure(t *testing.T) {
	synctest.Test(t, testHandleTenantUpdateRestartFailureImpl)
}

func testHandleTenantUpdateRestartFailureImpl(t *testing.T) {
	const failingCheckID = int64(6001)

	var failNextRestart atomic.Bool

	factory := func(ctx context.Context, check model.Check, publisher pusher.Publisher, probe sm.Probe,
		features feature.Collection,
		logger zerolog.Logger,
		metrics scraper.Metrics,
		k6Runner k6runner.Runner,
		labelsLimiter scraper.LabelsLimiter,
		telemeter *telemetry.Telemeter,
		secretStore secrets.SecretProvider,
		cals scraper.TenantCals,
		labellingMode scraper.TenantLabelMode,
	) (*scraper.Scraper, error) {
		if check.Id == failingCheckID && failNextRestart.Load() {
			return nil, errors.New("synthetic factory failure for test")
		}

		return testScraperFactory(ctx, check, publisher, probe, features, logger, metrics, k6Runner, labelsLimiter, telemeter, secretStore, cals, labellingMode)
	}

	u, err := NewUpdater(
		UpdaterOptions{
			Conn:           new(grpc.ClientConn),
			PromRegisterer: prometheus.NewPedanticRegistry(),
			Publisher:      channelPublisher(make(chan pusher.Payload, 100)),
			TenantCh:       make(chan sm.Tenant, 10),
			Logger:         testhelper.Logger(t),
			ScraperFactory: factory,
		},
	)
	require.NoError(t, err)

	u.probe = &sm.Probe{Id: 100, Name: "test-probe"}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	newCheck := func(id int64) model.Check {
		var check model.Check

		err := check.FromSM(sm.Check{
			Id:        id,
			TenantId:  55,
			Frequency: 1000,
			Timeout:   1000,
			Target:    "127.0.0.1",
			Job:       "test-job",
			Probes:    []int64{1},
			Settings:  sm.CheckSettings{Ping: &sm.PingSettings{}},
		})
		require.NoError(t, err)

		return check
	}

	failingCheck := newCheck(failingCheckID)
	okCheck := newCheck(6002)

	require.NoError(t, u.handleCheckAdd(ctx, failingCheck))
	require.NoError(t, u.handleCheckAdd(ctx, okCheck))
	requireRegistryConsistent(t, u.scrapers)

	scraperPointer := func(id model.GlobalID) *scraper.Scraper {
		s, _ := u.scrapers.get(id)
		return s
	}

	beforeOK := scraperPointer(okCheck.GlobalID())
	beforeFailing := scraperPointer(failingCheck.GlobalID())

	require.NotNil(t, beforeOK)
	require.NotNil(t, beforeFailing)
	require.Equal(t, 0.0, testutil.ToFloat64(u.metrics.changeErrorsCounter.WithLabelValues("update")))

	// Arm the failure only now, so both checks started cleanly above and
	// the only failure this test exercises is the one inside
	// handleTenantUpdate's restart loop.
	failNextRestart.Store(true)

	u.handleTenantUpdate(ctx, sm.Tenant{Id: 55, LabelMode: sm.LabelMode_LABEL_MODE_DUAL_WRITE})
	requireRegistryConsistent(t, u.scrapers)

	// restartCheck builds the replacement before touching the old scraper,
	// so a construction failure must leave the old scraper registered and
	// running: better a scraper on the stale label mode than a check with
	// no scraper and no retry path.
	afterFailing := scraperPointer(failingCheck.GlobalID())
	require.NotNil(t, afterFailing)
	require.Same(t, beforeFailing, afterFailing, "a failed restart must leave the old scraper in place")

	// The other check in the same tenant must still have been restarted:
	// one check's restart failure must not abort the loop over the rest.
	afterOK := scraperPointer(okCheck.GlobalID())
	require.NotNil(t, afterOK)
	require.NotSame(t, beforeOK, afterOK, "okCheck's scraper should still be restarted despite failingCheck's error")

	require.Equal(t, 1.0, testutil.ToFloat64(u.metrics.changeErrorsCounter.WithLabelValues("update")),
		"a restart failure must increment changeErrorsCounter the same way other failure paths in this file do")

	// The surviving old scraper still counts as running.
	require.Equal(t, 2.0, testutil.ToFloat64(u.metrics.runningScrapers))

	// Clean up: the failing check still has its (old) scraper to delete.
	require.NoError(t, u.handleCheckDelete(ctx, okCheck))
	require.NoError(t, u.handleCheckDelete(ctx, failingCheck))
	requireRegistryConsistent(t, u.scrapers)
	synctest.Wait()
}

// TestHandleTenantUpdateRegionEncodedIDs pins the ID-space agreement that
// tenant isolation rests on: for a check delivered with region-encoded
// (global, negative) wire IDs, the registry's tenant index key
// (check.GlobalTenantID()) must be the same value as the sm.Tenant.Id the
// API sends for that tenant. A regression indexing by the decoded local
// tenant ID (GlobalID(check.TenantId)) must fail this test.
func TestHandleTenantUpdateRegionEncodedIDs(t *testing.T) {
	synctest.Test(t, testHandleTenantUpdateRegionEncodedIDsImpl)
}

func testHandleTenantUpdateRegionEncodedIDsImpl(t *testing.T) {
	u, err := NewUpdater(
		UpdaterOptions{
			Conn:           new(grpc.ClientConn),
			PromRegisterer: prometheus.NewPedanticRegistry(),
			Publisher:      channelPublisher(make(chan pusher.Payload, 100)),
			TenantCh:       make(chan sm.Tenant, 10),
			Logger:         testhelper.Logger(t),
			ScraperFactory: testScraperFactory,
		},
	)
	require.NoError(t, err)

	u.probe = &sm.Probe{Id: 100, Name: "test-probe"}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Global wire IDs per pkg/pb/synthetic_monitoring/ids.go:
	// -(localID*1000 + regionID). Check 5001 and tenant 42, both region 2.
	const (
		globalCheckID  = int64(-5001002)
		globalTenantID = int64(-42002)
	)

	var check model.Check

	require.NoError(t, check.FromSM(sm.Check{
		Id:        globalCheckID,
		TenantId:  globalTenantID,
		Frequency: 1000,
		Timeout:   1000,
		Target:    "127.0.0.1",
		Job:       "test-job",
		Probes:    []int64{1},
		Settings:  sm.CheckSettings{Ping: &sm.PingSettings{}},
	}))

	// FromSM decodes the wire IDs to local ones; the global forms are
	// recovered through the region. Pin all of it so a change to either
	// side of the agreement shows up here, not as a silent non-restart.
	require.Equal(t, int64(5001), check.Id)
	require.Equal(t, 2, check.RegionId)
	require.Equal(t, int64(42), check.TenantId)
	require.Equal(t, model.GlobalID(globalCheckID), check.GlobalID())
	require.Equal(t, model.GlobalID(globalTenantID), check.GlobalTenantID())

	require.NoError(t, u.handleCheckAdd(ctx, check))
	requireRegistryConsistent(t, u.scrapers)

	before, found := u.scrapers.get(check.GlobalID())
	require.True(t, found)

	// The API addresses the tenant by its global wire ID.
	u.handleTenantUpdate(ctx, sm.Tenant{Id: globalTenantID, LabelMode: sm.LabelMode_LABEL_MODE_DUAL_WRITE})
	requireRegistryConsistent(t, u.scrapers)

	after, found := u.scrapers.get(check.GlobalID())
	require.True(t, found)
	require.NotSame(t, before, after, "a region-encoded tenant ID must reach the scrapers indexed under it")

	require.NoError(t, u.handleCheckDelete(ctx, check))
	requireRegistryConsistent(t, u.scrapers)
	synctest.Wait()
}

// TestHandleChangeBatchTenantWiring drives handleChangeBatch directly: a
// delta batch carrying a tenant restarts that tenant's scrapers via
// handleTenantUpdate, while a classic first batch (IsDeltaFirstBatch
// false) never looks at changes.Tenants at all — handleFirstBatch ignores
// them (pre-existing behavior, pinned here).
func TestHandleChangeBatchTenantWiring(t *testing.T) {
	synctest.Test(t, testHandleChangeBatchTenantWiringImpl)
}

func testHandleChangeBatchTenantWiringImpl(t *testing.T) {
	tenantCh := make(chan sm.Tenant, 10)

	u, err := NewUpdater(
		UpdaterOptions{
			Conn:           new(grpc.ClientConn),
			PromRegisterer: prometheus.NewPedanticRegistry(),
			Publisher:      channelPublisher(make(chan pusher.Payload, 100)),
			TenantCh:       tenantCh,
			Logger:         testhelper.Logger(t),
			ScraperFactory: testScraperFactory,
		},
	)
	require.NoError(t, err)

	u.probe = &sm.Probe{Id: 100, Name: "test-probe"}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	smCheck := sm.Check{
		Id:        7001,
		TenantId:  42,
		Frequency: 1000,
		Timeout:   1000,
		Target:    "127.0.0.1",
		Job:       "test-job",
		Probes:    []int64{1},
		Settings:  sm.CheckSettings{Ping: &sm.PingSettings{}},
	}

	var check model.Check
	require.NoError(t, check.FromSM(smCheck))

	// deliver the check through a delta batch, the way production would
	u.handleChangeBatch(ctx, &sm.Changes{
		Checks: []sm.CheckChange{{Operation: sm.CheckOperation_CHECK_ADD, Check: smCheck}},
	}, false)
	requireRegistryConsistent(t, u.scrapers)

	p0, found := u.scrapers.get(check.GlobalID())
	require.True(t, found)

	// a delta batch carrying a tenant must reach handleTenantUpdate
	u.handleChangeBatch(ctx, &sm.Changes{
		Tenants: []sm.Tenant{{Id: 42, LabelMode: sm.LabelMode_LABEL_MODE_DUAL_WRITE}},
	}, false)
	requireRegistryConsistent(t, u.scrapers)

	p1, found := u.scrapers.get(check.GlobalID())
	require.True(t, found)
	require.NotSame(t, p0, p1, "a delta batch with a tenant mode change must restart that tenant's scrapers")

	// repeating the same mode in another delta batch restarts nothing
	u.handleChangeBatch(ctx, &sm.Changes{
		Tenants: []sm.Tenant{{Id: 42, LabelMode: sm.LabelMode_LABEL_MODE_DUAL_WRITE}},
	}, false)
	requireRegistryConsistent(t, u.scrapers)

	p2, found := u.scrapers.get(check.GlobalID())
	require.True(t, found)
	require.Same(t, p1, p2)

	// a classic first batch ignores changes.Tenants: no restart and no
	// mode recording, even though the mode differs from the recorded one
	u.handleChangeBatch(ctx, &sm.Changes{
		Checks:  []sm.CheckChange{{Operation: sm.CheckOperation_CHECK_ADD, Check: smCheck}},
		Tenants: []sm.Tenant{{Id: 42, LabelMode: sm.LabelMode_LABEL_MODE_UNPREFIXED}},
	}, true)
	requireRegistryConsistent(t, u.scrapers)

	p3, found := u.scrapers.get(check.GlobalID())
	require.True(t, found)
	require.Same(t, p1, p3, "handleFirstBatch must not act on tenants")

	// ...and because the first batch recorded nothing, the same mode in a
	// later delta batch still counts as a change
	u.handleChangeBatch(ctx, &sm.Changes{
		Tenants: []sm.Tenant{{Id: 42, LabelMode: sm.LabelMode_LABEL_MODE_UNPREFIXED}},
	}, false)
	requireRegistryConsistent(t, u.scrapers)

	p4, found := u.scrapers.get(check.GlobalID())
	require.True(t, found)
	require.NotSame(t, p3, p4)

	// only the three delta batches forwarded their tenant to tenantCh
	require.Len(t, tenantCh, 3)

	require.NoError(t, u.handleCheckDelete(ctx, check))
	requireRegistryConsistent(t, u.scrapers)
	synctest.Wait()
}

// TestHandleFirstBatchSweep covers the reconnect sweep at the Updater
// level: scrapers absent from the first batch are stopped and dropped
// (registry index included), survivors with an unchanged config version
// keep their scraper.
func TestHandleFirstBatchSweep(t *testing.T) {
	synctest.Test(t, testHandleFirstBatchSweepImpl)
}

func testHandleFirstBatchSweepImpl(t *testing.T) {
	u, err := NewUpdater(
		UpdaterOptions{
			Conn:           new(grpc.ClientConn),
			PromRegisterer: prometheus.NewPedanticRegistry(),
			Publisher:      channelPublisher(make(chan pusher.Payload, 100)),
			TenantCh:       make(chan sm.Tenant, 10),
			Logger:         testhelper.Logger(t),
			ScraperFactory: testScraperFactory,
		},
	)
	require.NoError(t, err)

	u.probe = &sm.Probe{Id: 100, Name: "test-probe"}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	newSMCheck := func(id, tenantID int64) sm.Check {
		return sm.Check{
			Id:        id,
			TenantId:  tenantID,
			Frequency: 1000,
			Timeout:   1000,
			Target:    "127.0.0.1",
			Job:       "test-job",
			Probes:    []int64{1},
			Settings:  sm.CheckSettings{Ping: &sm.PingSettings{}},
		}
	}

	smKept := newSMCheck(8001, 42)
	smSwept := newSMCheck(8002, 43)

	var kept, swept model.Check

	require.NoError(t, kept.FromSM(smKept))
	require.NoError(t, swept.FromSM(smSwept))

	require.NoError(t, u.handleCheckAdd(ctx, kept))
	require.NoError(t, u.handleCheckAdd(ctx, swept))
	requireRegistryConsistent(t, u.scrapers)
	require.Equal(t, 2.0, testutil.ToFloat64(u.metrics.runningScrapers))

	keptBefore, found := u.scrapers.get(kept.GlobalID())
	require.True(t, found)

	// reconnect: the server only knows about one of the two checks
	u.handleFirstBatch(ctx, &sm.Changes{
		Checks: []sm.CheckChange{{Operation: sm.CheckOperation_CHECK_ADD, Check: smKept}},
	})
	requireRegistryConsistent(t, u.scrapers)

	_, found = u.scrapers.get(swept.GlobalID())
	require.False(t, found, "a scraper absent from the first batch must be swept")

	keptAfter, found := u.scrapers.get(kept.GlobalID())
	require.True(t, found)
	require.Same(t, keptBefore, keptAfter, "a survivor with an unchanged config version must not be restarted")

	require.Equal(t, 1.0, testutil.ToFloat64(u.metrics.runningScrapers))

	require.NoError(t, u.handleCheckDelete(ctx, kept))
	requireRegistryConsistent(t, u.scrapers)
	synctest.Wait()
}

func TestCheckHandlerProbeValidation(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		opts          UpdaterOptions
		probe         sm.Probe
		expectedError error
	}{
		"has K6 when required for scripted checks": {
			expectedError: nil,
			opts: UpdaterOptions{
				Conn:           new(grpc.ClientConn),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				Publisher:      channelPublisher(make(chan pusher.Payload)),
				TenantCh:       make(chan<- sm.Tenant),
				Logger:         testhelper.Logger(t),
				K6Runner:       noopRunner{},
			},
			probe: sm.Probe{Id: 100, Name: "test-probe", Capabilities: &sm.Probe_Capabilities{
				DisableScriptedChecks: false,
				DisableBrowserChecks:  true,
			}},
		},
		"missing K6 when required for scripted checks": {
			expectedError: errCapabilityK6Missing,
			opts: UpdaterOptions{
				Conn:           new(grpc.ClientConn),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				Publisher:      channelPublisher(make(chan pusher.Payload)),
				TenantCh:       make(chan<- sm.Tenant),
				Logger:         testhelper.Logger(t),
			},
			probe: sm.Probe{Id: 100, Name: "test-probe", Capabilities: &sm.Probe_Capabilities{
				DisableScriptedChecks: false,
				DisableBrowserChecks:  true,
			}},
		},
		"has K6 when required for browser checks": {
			expectedError: nil,
			opts: UpdaterOptions{
				Conn:           new(grpc.ClientConn),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				Publisher:      channelPublisher(make(chan pusher.Payload)),
				TenantCh:       make(chan<- sm.Tenant),
				Logger:         testhelper.Logger(t),
				K6Runner:       noopRunner{},
			},
			probe: sm.Probe{Id: 100, Name: "test-probe", Capabilities: &sm.Probe_Capabilities{
				DisableScriptedChecks: true,
				DisableBrowserChecks:  false,
			}},
		},
		"missing K6 when required for browser checks": {
			expectedError: errCapabilityK6Missing,
			opts: UpdaterOptions{
				Conn:           new(grpc.ClientConn),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				Publisher:      channelPublisher(make(chan pusher.Payload)),
				TenantCh:       make(chan<- sm.Tenant),
				Logger:         testhelper.Logger(t),
			},
			probe: sm.Probe{Id: 100, Name: "test-probe", Capabilities: &sm.Probe_Capabilities{
				DisableScriptedChecks: true,
				DisableBrowserChecks:  false,
			}},
		},
		"has K6 when required for scripted and browser checks": {
			expectedError: nil,
			opts: UpdaterOptions{
				Conn:           new(grpc.ClientConn),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				Publisher:      channelPublisher(make(chan pusher.Payload)),
				TenantCh:       make(chan<- sm.Tenant),
				Logger:         testhelper.Logger(t),
				K6Runner:       noopRunner{},
			},
			probe: sm.Probe{Id: 100, Name: "test-probe", Capabilities: &sm.Probe_Capabilities{
				DisableScriptedChecks: false,
				DisableBrowserChecks:  false,
			}},
		},
		"missing K6 when required for scripted and browser checks": {
			expectedError: errCapabilityK6Missing,
			opts: UpdaterOptions{
				Conn:           new(grpc.ClientConn),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				Publisher:      channelPublisher(make(chan pusher.Payload)),
				TenantCh:       make(chan<- sm.Tenant),
				Logger:         testhelper.Logger(t),
			},
			probe: sm.Probe{Id: 100, Name: "test-probe", Capabilities: &sm.Probe_Capabilities{
				DisableScriptedChecks: false,
				DisableBrowserChecks:  false,
			}},
		},
		"has K6 but not required": {
			expectedError: nil,
			opts: UpdaterOptions{
				Conn:           new(grpc.ClientConn),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				Publisher:      channelPublisher(make(chan pusher.Payload)),
				TenantCh:       make(chan<- sm.Tenant),
				Logger:         testhelper.Logger(t),
				K6Runner:       noopRunner{},
			},
			probe: sm.Probe{Id: 100, Name: "test-probe", Capabilities: &sm.Probe_Capabilities{
				DisableScriptedChecks: true,
				DisableBrowserChecks:  true,
			}},
		},
		"missing K6 but not required": {
			expectedError: nil,
			opts: UpdaterOptions{
				Conn:           new(grpc.ClientConn),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				Publisher:      channelPublisher(make(chan pusher.Payload)),
				TenantCh:       make(chan<- sm.Tenant),
				Logger:         testhelper.Logger(t),
			},
			probe: sm.Probe{Id: 100, Name: "test-probe", Capabilities: &sm.Probe_Capabilities{
				DisableScriptedChecks: true,
				DisableBrowserChecks:  true,
			}},
		},
		"missing K6 when required by default": {
			expectedError: errCapabilityK6Missing,
			opts: UpdaterOptions{
				Conn:           new(grpc.ClientConn),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				Publisher:      channelPublisher(make(chan pusher.Payload)),
				TenantCh:       make(chan<- sm.Tenant),
				Logger:         testhelper.Logger(t),
			},
			probe: sm.Probe{Id: 100, Name: "test-probe"},
		},
		"has K6 when required by default": {
			expectedError: nil,
			opts: UpdaterOptions{
				Conn:           new(grpc.ClientConn),
				PromRegisterer: prometheus.NewPedanticRegistry(),
				Publisher:      channelPublisher(make(chan pusher.Payload)),
				TenantCh:       make(chan<- sm.Tenant),
				Logger:         testhelper.Logger(t),
				K6Runner:       noopRunner{},
			},
			probe: sm.Probe{Id: 100, Name: "test-probe"},
		},
	}

	for testName, tc := range testcases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			u, err := NewUpdater(tc.opts)
			require.NoError(t, err)

			err = u.validateProbeCapabilities(tc.probe.Capabilities)

			if tc.expectedError != nil {
				require.Error(t, err, tc.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

type testProber struct{}

func (testProber) Name() string {
	return "test-prober"
}

func (testProber) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger, _ string) (bool, float64) {
	return false, 0
}

type testProbeFactory struct{}

func (f testProbeFactory) New(ctx context.Context, logger zerolog.Logger, check model.Check) (prober.Prober, string, error) {
	return testProber{}, check.Target, nil
}

type testLabelsLimiter struct {
	metricLabelsLimit int
	logLabelsLimit    int
}

func (l testLabelsLimiter) MetricLabels(ctx context.Context, tenantID model.GlobalID) (int, error) {
	return l.metricLabelsLimit, nil
}

func (l testLabelsLimiter) LogLabels(ctx context.Context, tenantID model.GlobalID) (int, error) {
	return l.logLabelsLimit, nil
}

type testLabellingMode struct{}

func (l testLabellingMode) ForTenant(ctx context.Context, tenantID model.GlobalID) (sm.LabelMode, error) {
	return sm.LabelMode_LABEL_MODE_PREFIXED, nil
}

func testScraperFactory(ctx context.Context, check model.Check, publisher pusher.Publisher, _ sm.Probe,
	_ feature.Collection,
	logger zerolog.Logger,
	metrics scraper.Metrics,
	k6Runner k6runner.Runner,
	labelsLimiter scraper.LabelsLimiter,
	telemeter *telemetry.Telemeter,
	secretStore secrets.SecretProvider,
	cals scraper.TenantCals,
	labellingMode scraper.TenantLabelMode,
) (*scraper.Scraper, error) {
	return scraper.NewWithOpts(
		ctx,
		check,
		scraper.ScraperOpts{
			Logger:        logger,
			ProbeFactory:  testProbeFactory{},
			Publisher:     publisher,
			Metrics:       metrics,
			LabelsLimiter: testLabelsLimiter{},
			LabellingMode: testLabellingMode{},
			Telemeter:     telemeter,
		},
	)
}

var _ scraper.Factory = testScraperFactory

type channelPublisher chan pusher.Payload

func (c channelPublisher) Publish(payload pusher.Payload) {
	c <- payload
}

type noopRunner struct{}

func (noopRunner) WithLogger(logger *zerolog.Logger) k6runner.Runner {
	var r noopRunner
	return r
}

func (noopRunner) Run(ctx context.Context, script k6runner.Script, secretStore k6runner.SecretStore, _ string) (*k6runner.RunResponse, error) {
	return &k6runner.RunResponse{}, nil
}

func (noopRunner) Versions(_ context.Context) <-chan []string {
	return nil // Blocks forever if read.
}

type testBackoff time.Duration

func (b *testBackoff) Reset() {
	*b = 0
}

func (b testBackoff) Duration() time.Duration {
	return time.Duration(b)
}

// TestHandleError tests the handleError function. It considers the errors that
// might be returned from the loop method.
func TestHandleError(t *testing.T) {
	ctx, cancel := testhelper.Context(context.Background(), t)
	defer cancel()

	logger := testhelper.Logger(t)

	t.Run("no error", func(t *testing.T) {
		backoff := testBackoff(1)
		done, err := handleError(ctx, logger, &backoff, false, nil)
		require.True(t, done)
		require.NoError(t, err)
		require.NotZero(t, backoff)
	})

	t.Run("context cancelled", func(t *testing.T) {
		backoff := testBackoff(1)
		done, err := handleError(ctx, logger, &backoff, false, fmt.Errorf("wrapped: %w", context.Canceled))
		require.True(t, done)
		require.NoError(t, err)
		require.NotZero(t, backoff)
	})

	t.Run("k6 capability missing", func(t *testing.T) {
		backoff := testBackoff(1)
		done, err := handleError(ctx, logger, &backoff, false, errCapabilityK6Missing)
		require.True(t, done)
		require.ErrorIs(t, err, errCapabilityK6Missing)
		require.NotZero(t, backoff)
	})

	t.Run("incompatible API", func(t *testing.T) {
		backoff := testBackoff(1)
		done, err := handleError(ctx, logger, &backoff, false, errIncompatibleApi)
		require.True(t, done)
		require.ErrorIs(t, err, errIncompatibleApi)
		require.NotZero(t, backoff)
	})

	t.Run("not authorized", func(t *testing.T) {
		backoff := testBackoff(1)
		done, err := handleError(ctx, logger, &backoff, false, errNotAuthorized)
		require.True(t, done)
		require.ErrorIs(t, err, errNotAuthorized)
		require.NotZero(t, backoff)
	})

	t.Run("transport closing - not connected", func(t *testing.T) {
		backoff := testBackoff(1)
		done, err := handleError(ctx, logger, &backoff, false, errTransportClosing)
		require.False(t, done)
		require.NoError(t, err)
		require.NotZero(t, backoff)
	})

	t.Run("transport closing - not connected - cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()

		backoff := testBackoff(time.Second)
		done, err := handleError(ctx, logger, &backoff, false, errTransportClosing)
		require.True(t, done)
		require.ErrorIs(t, err, context.Canceled)
		require.NotZero(t, backoff)
	})

	t.Run("transport closing - connected", func(t *testing.T) {
		backoff := testBackoff(1)
		done, err := handleError(ctx, logger, &backoff, true, errTransportClosing)
		require.False(t, done)
		require.NoError(t, err)
		require.Zero(t, backoff)
	})

	t.Run("probe unregistered - not connected", func(t *testing.T) {
		backoff := testBackoff(1)
		done, err := handleError(ctx, logger, &backoff, false, errProbeUnregistered)
		require.False(t, done)
		require.NoError(t, err)
		require.NotZero(t, backoff)
	})

	t.Run("probe unregistered - connected", func(t *testing.T) {
		backoff := testBackoff(1)
		done, err := handleError(ctx, logger, &backoff, true, errProbeUnregistered)
		require.False(t, done)
		require.NoError(t, err)
		require.Zero(t, backoff)
	})
}

func TestProbeTenantCh(t *testing.T) {
	t.Run("sends probe tenant ID after registration", func(t *testing.T) {
		probeTenantCh := make(chan *sm.Probe, 1)
		u, err := NewUpdater(UpdaterOptions{
			Conn:           new(grpc.ClientConn),
			PromRegisterer: prometheus.NewPedanticRegistry(),
			Publisher:      channelPublisher(make(chan pusher.Payload)),
			TenantCh:       make(chan<- sm.Tenant),
			ProbeCh:        probeTenantCh,
			Logger:         testhelper.Logger(t),
		})

		require.NoError(t, err)

		u.probe = &sm.Probe{Id: 1, TenantId: 42}
		u.notifyProbeTenant()

		got := <-probeTenantCh
		require.Equal(t, u.probe.TenantId, got.TenantId)
	})

	t.Run("only sends once across multiple registrations", func(t *testing.T) {
		probeCh := make(chan *sm.Probe, 1)
		u, err := NewUpdater(UpdaterOptions{
			Conn:           new(grpc.ClientConn),
			PromRegisterer: prometheus.NewPedanticRegistry(),
			Publisher:      channelPublisher(make(chan pusher.Payload)),
			TenantCh:       make(chan<- sm.Tenant),
			ProbeCh:        probeCh,
			Logger:         testhelper.Logger(t),
		})

		require.NoError(t, err)

		u.probe = &sm.Probe{Id: 1, TenantId: 42}
		u.notifyProbeTenant()

		u.probe = &sm.Probe{Id: 2, TenantId: 43}
		u.notifyProbeTenant()

		// Confirm that the returned value is the first probe id sent to the channel
		got := <-probeCh
		require.Equal(t, got.TenantId, int64(42))

		// Confirm no second send
		_, ok := <-probeCh
		require.False(t, ok)
	})

	t.Run("nil channel is safe when feature disabled", func(t *testing.T) {
		u, err := NewUpdater(UpdaterOptions{
			Conn:           new(grpc.ClientConn),
			PromRegisterer: prometheus.NewPedanticRegistry(),
			Publisher:      channelPublisher(make(chan pusher.Payload)),
			TenantCh:       make(chan<- sm.Tenant),
			Logger:         testhelper.Logger(t),
		})

		require.NoError(t, err)

		u.probe = &sm.Probe{Id: 1, TenantId: 42}

		require.NotPanics(t, func() {
			u.notifyProbeTenant()
		})
	})
}
