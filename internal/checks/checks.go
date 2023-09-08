package checks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/logproto"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/scraper"
	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type Error string

func (e Error) Error() string { return string(e) }

type TransientError Error

func (e TransientError) Error() string { return string(e) }

const (
	errNotAuthorized     = Error("probe not authorized")
	errTransportClosing  = TransientError("transport closing")
	errProbeUnregistered = TransientError("probe no longer registered")
	errIncompatibleApi   = Error("API does not support required features")
)

// Backoffer defines an interface to provide backoff durations.
//
// The implementation of this interface SHOULD NOT perform the actual
// sleep, but rather return the duration to sleep.
type Backoffer interface {
	// Reset causes the backoff provider to go back its initial
	// state, before any calls to Duration() were made.
	Reset()
	// Duration returns the duration to sleep.
	Duration() time.Duration
}

// Updater represents a probe along with the collection of scrapers
// running on that probe and it manages the configuration for
// blackbox-exporter that corresponds to the collection of scrapers.
type Updater struct {
	api            apiInfo
	logger         zerolog.Logger
	features       feature.Collection
	backoff        Backoffer
	publisher      pusher.Publisher
	tenantCh       chan<- sm.Tenant
	IsConnected    func(bool)
	probe          *sm.Probe
	scrapersMutex  sync.Mutex
	scrapers       map[model.GlobalID]*scraper.Scraper
	metrics        metrics
	k6Runner       k6runner.Runner
	scraperFactory func(context.Context, model.Check, pusher.Publisher, sm.Probe, zerolog.Logger, scraper.Incrementer, scraper.IncrementerVec, k6runner.Runner) (*scraper.Scraper, error)
}

type apiInfo struct {
	conn *grpc.ClientConn
}

type metrics struct {
	changeErrorsCounter *prometheus.CounterVec
	changesCounter      *prometheus.CounterVec
	connectionStatus    prometheus.Gauge
	probeInfo           *prometheus.GaugeVec
	runningScrapers     *prometheus.GaugeVec
	scrapeErrorCounter  *prometheus.CounterVec
	scrapesCounter      *prometheus.CounterVec
}

type TimeSeries = []prompb.TimeSeries
type Streams = []logproto.Stream

type UpdaterOptions struct {
	Conn           *grpc.ClientConn
	Logger         zerolog.Logger
	Backoff        Backoffer
	Publisher      pusher.Publisher
	TenantCh       chan<- sm.Tenant
	IsConnected    func(bool)
	PromRegisterer prometheus.Registerer
	Features       feature.Collection
	K6Runner       k6runner.Runner
	ScraperFactory func(context.Context, model.Check, pusher.Publisher, sm.Probe, zerolog.Logger, scraper.Incrementer, scraper.IncrementerVec, k6runner.Runner) (*scraper.Scraper, error)
}

func NewUpdater(opts UpdaterOptions) (*Updater, error) {
	changesCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sm_agent",
		Subsystem: "updater",
		Name:      "changes_total",
		Help:      "Total number of changes processed.",
	}, []string{
		"type",
	})

	if err := opts.PromRegisterer.Register(changesCounter); err != nil {
		return nil, err
	}

	changeErrorsCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sm_agent",
		Subsystem: "updater",
		Name:      "change_errors_total",
		Help:      "Total number of errors during change processing.",
	}, []string{
		"type",
	})

	if err := opts.PromRegisterer.Register(changeErrorsCounter); err != nil {
		return nil, err
	}

	runningScrapers := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "sm_agent",
		Subsystem: "updater",
		Name:      "scrapers_total",
		Help:      "Total number of running scrapers.",
	}, []string{
		"type",
	})

	if err := opts.PromRegisterer.Register(runningScrapers); err != nil {
		return nil, err
	}

	scrapesCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sm_agent",
		Subsystem: "scraper",
		Name:      "operations_total",
		Help:      "Total number of scrape operations performed by type.",
	}, []string{
		"type",
		"tenantId",
		"regionId",
	})

	if err := opts.PromRegisterer.Register(scrapesCounter); err != nil {
		return nil, err
	}

	scrapeErrorCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sm_agent",
		Subsystem: "scraper",
		Name:      "errors_total",
		Help:      "Total number of scraper errors by type and status.",
	}, []string{
		"type",
		"source",
		"tenantId",
		"regionId",
	})

	if err := opts.PromRegisterer.Register(scrapeErrorCounter); err != nil {
		return nil, err
	}

	connectionStatusGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "sm_agent",
		Subsystem: "api_connection",
		Name:      "status",
		Help:      "API connection status.",
	})

	if err := opts.PromRegisterer.Register(connectionStatusGauge); err != nil {
		return nil, err
	}

	connectionStatusGauge.Set(0)

	probeInfoGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "sm_agent",
		Name:      "info",
		Help:      "Agent information.",
	}, []string{
		"id",
		"name",
		"version",
		"commit",
		"buildstamp",
	})

	if err := opts.PromRegisterer.Register(probeInfoGauge); err != nil {
		return nil, err
	}

	scraperFactory := scraper.New
	if opts.ScraperFactory != nil {
		scraperFactory = opts.ScraperFactory
	}

	return &Updater{
		api: apiInfo{
			conn: opts.Conn,
		},
		logger:         opts.Logger,
		features:       opts.Features,
		backoff:        opts.Backoff,
		publisher:      opts.Publisher,
		tenantCh:       opts.TenantCh,
		IsConnected:    opts.IsConnected,
		scrapers:       make(map[model.GlobalID]*scraper.Scraper),
		k6Runner:       opts.K6Runner,
		scraperFactory: scraperFactory,
		metrics: metrics{
			changeErrorsCounter: changeErrorsCounter,
			changesCounter:      changesCounter,
			connectionStatus:    connectionStatusGauge,
			probeInfo:           probeInfoGauge,
			runningScrapers:     runningScrapers,
			scrapeErrorCounter:  scrapeErrorCounter,
			scrapesCounter:      scrapesCounter,
		},
	}, nil
}

func (c *Updater) Run(ctx context.Context) error {
	c.backoff.Reset()

	for {
		var transientErr *TransientError

		wasConnected, err := c.loop(ctx)

		c.logger.Info().
			Err(err).
			Bool("was_connected", wasConnected).
			Str("connection_state", c.api.conn.GetState().String()).
			Msg("broke out of loop")

		switch {
		case err == nil:
			return nil

		case errors.As(err, &transientErr):
			c.logger.Warn().
				Err(err).
				Str("connection_state", c.api.conn.GetState().String()).
				Msg("transient error, trying to reconnect")

			if err := sleepCtx(ctx, c.backoff.Duration()); err != nil {
				return err
			}

			if wasConnected {
				c.backoff.Reset()
			}

			continue

		case errors.Is(err, errNotAuthorized):
			// our token is invalid, bail out?
			c.logger.Error().
				Err(err).
				Str("connection_state", c.api.conn.GetState().String()).
				Msg("cannot connect, bailing out")
			return err

		case errors.Is(err, errIncompatibleApi):
			// API server doesn't support required features.
			c.logger.Error().
				Err(err).
				Str("connection_state", c.api.conn.GetState().String()).
				Msg("cannot connect, bailing out")
			return err

		case errors.Is(err, context.Canceled):
			// context was cancelled, clean up
			c.logger.Error().
				Err(err).
				Str("connection_state", c.api.conn.GetState().String()).
				Msg("context cancelled, closing updater")
			return nil

		default:
			c.logger.Warn().
				Err(err).
				Str("connection_state", c.api.conn.GetState().String()).
				Msg("handling check changes")

			// TODO(mem): this might be a transient error (e.g. bad connection). We probably need to
			// fine-tune GRPPC's backoff parameters. We might also need to keep count of the reconnects, and
			// give up if they hit some threshold?
			if err := sleepCtx(ctx, c.backoff.Duration()); err != nil {
				return err
			}
		}
	}
}

func (c *Updater) loop(ctx context.Context) (bool, error) {
	connected := false

	c.logger.Info().Msg("fetching check configuration from synthetic-monitoring-api")

	client := sm.NewChecksClient(c.api.conn)

	grpcErrorHandler := func(action string, err error) error {
		status, ok := status.FromError(err)
		c.logger.Error().Err(err).Str("action", action).Uint32("code", uint32(status.Code())).Msg(status.Message())

		switch {
		case !ok:
			return fmt.Errorf("%s: %w", action, err)

		case status.Code() == codes.Canceled:
			// either we were told to shut down
			return context.Canceled

		case status.Message() == "transport is closing":
			// the other end is shutting down
			return errTransportClosing

		case status.Code() == codes.PermissionDenied:
			return errNotAuthorized

		case status.Code() == codes.Unimplemented:
			return errIncompatibleApi

		default:
			return status.Err()
		}
	}

	result, err := client.RegisterProbe(ctx, &sm.ProbeInfo{Version: version.Short(), Commit: version.Commit(), Buildstamp: version.Buildstamp()})
	if err != nil {
		return connected, grpcErrorHandler("registering probe with synthetic-monitoring-api", err)
	}

	switch result.Status.Code {
	case sm.StatusCode_OK:
		// continue

	case sm.StatusCode_NOT_AUTHORIZED:
		return connected, errNotAuthorized

	default:
		return connected, fmt.Errorf("registering probe with synthetic-monitoring-api, response: %s", result.Status.Message)
	}

	c.probe = &result.Probe

	logger := c.logger.With().Int64("probe_id", c.probe.Id).Logger()

	logger.Info().Str("probe_name", c.probe.Name).Msg("registered probe with synthetic-monitoring-api")

	c.metrics.connectionStatus.Set(1)
	defer c.metrics.connectionStatus.Set(0)

	// true indicates that probe is connected to API
	connected = true
	c.IsConnected(true)
	defer c.IsConnected(false)

	// this is constant throughout the life of the probe, but since
	// we don't know the probe's id or name until this point, set it
	// here.
	c.metrics.probeInfo.Reset()
	c.metrics.probeInfo.With(map[string]string{
		"id":         strconv.FormatInt(c.probe.Id, 10),
		"name":       c.probe.Name,
		"version":    version.Short(),
		"commit":     version.Commit(),
		"buildstamp": version.Buildstamp(),
	}).Set(1)

	// groupCtx is used to coordinate shutting down all the
	// goroutines started here.
	g, groupCtx := errgroup.WithContext(ctx)

	// We get _another_ context from the signal handler that we can
	// use tell the GRPC client that we need to break out. We have
	// multiple ways of cancelling the context (another signal
	// elsewhere in the system communicated through the parent
	// context; cancelling the child context because we are
	// returning from this function; cancelling the new context
	// because the signal fired), so we need an additional way of
	// telling them apart.
	sigCtx, signalFired := installSignalHandler(groupCtx)

	errorHandler := func(err error, action string, signalFired *int32) error {
		switch {
		case err == nil:
			return nil

		case atomic.LoadInt32(signalFired) == 1:
			return errProbeUnregistered

		default:
			return grpcErrorHandler(action, err)
		}
	}

	knownChecks := sm.ProbeState{
		Checks: make([]sm.EntityRef, 0, len(c.scrapers)),
	}

	for cID, scraper := range c.scrapers {
		knownChecks.Checks = append(knownChecks.Checks, sm.EntityRef{
			Id:           int64(cID),
			LastModified: scraper.LastModified(),
		})
	}

	cc, err := client.GetChanges(sigCtx, &knownChecks)
	if err != nil {
		return connected, errorHandler(err, "requesting changes from synthetic-monitoring-api", signalFired)
	}

	// Run a ping to the GRPC server. Bail out if there's an error here.
	g.Go(func() error {
		err := ping(sigCtx, client)
		logger.Warn().Err(err).Msg("health check ping stopped")
		return err
	})

	g.Go(func() error {
		// processChanges uses the context in its first argument to
		// create scrapers. This means that cancelling that context
		// cancels all the running scrapers. That's why we are passing
		// the _original_ context, not sigCtx, so that scrapers are
		// _not_ stopped if the signal is trapped. We want scrapers to
		// continue running in case the agent is _not_ killed.
		err := c.processChanges(ctx, cc)
		logger.Warn().Err(err).Msg("processing changes stopped")
		return err
	})

	err = g.Wait()

	return connected, errorHandler(err, "getting changes from synthetic-monitoring-api", signalFired)
}

// ping will use the provided client to send a health signal to the GRPC
// server. Any error is returned, the caller should take the necessary
// steps to correct the problem.
func ping(ctx context.Context, client synthetic_monitoring.ChecksClient) error {
	var (
		req  synthetic_monitoring.PingRequest
		opts = []grpc.CallOption{
			grpc.WaitForReady(false),
		}
	)

	// Send one ping to try to figure out if the API understands
	// what we are trying to do.
	_, err := client.Ping(ctx, &req, opts...)
	if err != nil {
		status, ok := status.FromError(err)

		switch {
		case !ok:
			return fmt.Errorf("sending ping: %w", err)

		case status.Code() == codes.Unimplemented:
			// The API does not support this. Return without error.
			return nil

		default:
			return status.Err()
		}
	}

	req.Sequence++

	ticker := time.NewTicker(synthetic_monitoring.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			_, err := client.Ping(ctx, &req, opts...)
			if err != nil {
				return err
			}

			req.Sequence++
		}
	}
}

// installSignalHandler installs a signal handler for SIGUSR1.
//
// The returned context's Done channel is closed if the signal is
// delivered. To make it simpler to determine if the signal was
// delivered, a value of 1 is written to the location pointed to by the
// returned int32 pointer.
//
// If the provided context's Done channel is closed before the signal is
// delivered, the signal handler is removed and the returned context's
// Done channel is closed, too. It's the callers responsibility to
// cancel the provided context if it's no longer interested in the
// signal.
func installSignalHandler(ctx context.Context) (context.Context, *int32) {
	sigCtx, cancel := context.WithCancel(ctx)

	fired := new(int32)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)

	go func() {
		select {
		case <-sigCh:
			atomic.StoreInt32(fired, 1)
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()

	return sigCtx, fired
}

func (c *Updater) processChanges(ctx context.Context, cc sm.Checks_GetChangesClient) error {
	firstBatch := true

	for {
		select {
		case <-cc.Context().Done():
			return cc.Context().Err()

		case <-ctx.Done():
			return ctx.Err()

		default:
			switch msg, err := cc.Recv(); err {
			case nil:
				c.handleChangeBatch(ctx, msg, firstBatch)
				firstBatch = false

			case io.EOF:
				c.logger.Warn().Err(err).Msg("no more messages?")
				// XXX(mem): what happened here? The
				// other end told us there are no more
				// changes. Stop? Is it restarting?
				return nil

			default:
				return err
			}
		}
	}
}

func (c *Updater) handleCheckAdd(ctx context.Context, check model.Check) error {
	c.metrics.changesCounter.WithLabelValues("add").Inc()

	if err := check.Validate(); err != nil {
		return fmt.Errorf("invalid check: %w", err)
	}

	c.scrapersMutex.Lock()
	defer c.scrapersMutex.Unlock()

	if running, found := c.scrapers[check.GlobalID()]; found {
		// we can get here if the API sent us a check add twice:
		// once during the initial connection and another right
		// after that. The window for that is small, but it
		// exists.

		return fmt.Errorf("check with id %d already exists (version %s)", check.GlobalID(), running.ConfigVersion())
	}

	return c.addAndStartScraperWithLock(ctx, check)
}

func (c *Updater) handleCheckUpdate(ctx context.Context, check model.Check) error {
	c.metrics.changesCounter.WithLabelValues("update").Inc()

	if err := check.Validate(); err != nil {
		return fmt.Errorf("invalid check: %w", err)
	}

	c.scrapersMutex.Lock()
	defer c.scrapersMutex.Unlock()

	return c.handleCheckUpdateWithLock(ctx, check)
}

// handleCheckUpdateWithLock is the bottom half of handleCheckUpdate. It
// MUST be called with the scrapersMutex lock held.
func (c *Updater) handleCheckUpdateWithLock(ctx context.Context, check model.Check) error {
	cid := check.GlobalID()

	scraper, found := c.scrapers[cid]
	if !found {
		c.logger.Warn().Int64("check_id", check.Id).Int("region_id", check.RegionId).Msg("update request for an unknown check")
		return c.addAndStartScraperWithLock(ctx, check)
	}

	// this is the lazy way to update the scraper: tear everything
	// down, start it again.

	scraper.Stop()
	checkType := scraper.CheckType().String()
	delete(c.scrapers, cid)

	c.metrics.runningScrapers.WithLabelValues(checkType).Dec()

	return c.addAndStartScraperWithLock(ctx, check)
}

func (c *Updater) handleCheckDelete(ctx context.Context, check model.Check) error {
	c.metrics.changesCounter.WithLabelValues("delete").Inc()

	cid := check.GlobalID()

	c.scrapersMutex.Lock()
	defer c.scrapersMutex.Unlock()

	scraper, found := c.scrapers[cid]
	if !found {
		c.logger.Warn().Int64("check_id", check.Id).Int("region_id", check.RegionId).Msg("delete request for an unknown check")
		return errors.New("check not found")
	}

	scraper.Stop()
	checkType := scraper.CheckType().String()

	delete(c.scrapers, cid)

	c.metrics.runningScrapers.WithLabelValues(checkType).Dec()

	return nil
}

// handleFirstBatch takes a list of changes and adds them to the running set
// and stops any scrapers that shouldn't be running.
//
// When handling this, we don't know which scrapers the server thinks we are
// no longer running and which scrapers we are running. If we got
// disconnected, the server will send a bunch of ADD operations, and no DELETE
// or UPDATE ones. After we reconnect, we still have a bunch of running
// scrapers that the server might think we are NOT running, so build a list of
// checks the server sent our way and compare it with the list of checks we
// actually have (from the running scrapers). Remove anything that the server
// didn't send, because that means it didn't know we have those (they got
// deleted during the reconnect, and the server didn't send them).
//
// We have to do this exactly once per reconnect. It's up to the calling code
// to ensure this.
func (c *Updater) handleFirstBatch(ctx context.Context, changes *sm.Changes) {
	newChecks := make(map[model.GlobalID]struct{})

	c.scrapersMutex.Lock()
	defer c.scrapersMutex.Unlock()

	// add checks from the provided list
	for _, checkChange := range changes.Checks {
		c.logger.Debug().Interface("check change", checkChange).Msg("got check change")

		switch checkChange.Operation {
		case sm.CheckOperation_CHECK_ADD:
			var check model.Check
			if err := check.FromSM(checkChange.Check); err != nil {
				c.logger.Error().Err(err).Interface("check_change", checkChange).Msg("dropping check during add operation")
				continue
			}

			if err := c.handleInitialChangeAddWithLock(ctx, check); err != nil {
				c.metrics.changeErrorsCounter.WithLabelValues("add").Inc()
				c.logger.Error().Err(err).Int64("check_id", check.Id).Int("region_id", check.RegionId).
					Msg("adding check failed, dropping check")
				continue
			}

			// add this to the list of checks we have seen during
			// this operation
			newChecks[check.GlobalID()] = struct{}{}

		default:
			// we should never hit this because the first time we
			// connect the server will only send adds.
			c.logger.Warn().
				Interface("check", checkChange.Check).
				Str("operation", checkChange.Operation.String()).
				Msg("unexpected operation, dropping check change")
			continue
		}
	}

	// remove all the running scrapers that weren't sent with the first batch
	for id, scraper := range c.scrapers {
		if _, found := newChecks[id]; found {
			continue
		}

		cid, rid := model.GetLocalAndRegionIDs(id)
		c.logger.Debug().
			Int64("check_id", cid).
			Int("region_id", rid).
			Msg("stopping scraper during first batch handling")

		checkType := scraper.CheckType().String()
		scraper.Stop()

		delete(c.scrapers, id)

		c.metrics.runningScrapers.WithLabelValues(checkType).Dec()
	}
}

// handleCheckUpdateWithLock the specified check to the running checks.
//
// It deals with the case where this check is the product of a reconnection
// and changes the operation to an update if necessary.
//
// This function MUST be called with the scrapers mutex held.
func (c *Updater) handleInitialChangeAddWithLock(ctx context.Context, check model.Check) error {
	if running, found := c.scrapers[check.GlobalID()]; found {
		oldVersion := running.ConfigVersion()
		newVersion := check.ConfigVersion()

		if oldVersion == newVersion {
			// we already have this, skip
			//
			// XXX(mem): beware, the probe might have changed
			return nil
		}

		// transform this request into an update
		c.logger.Debug().Str("old_check_version", oldVersion).Str("new_check_version", newVersion).Msg("transforming add into update")
		return c.handleCheckUpdateWithLock(ctx, check)
	}

	c.metrics.changesCounter.WithLabelValues("add").Inc()

	if err := check.Validate(); err != nil {
		return err
	}

	if err := c.addAndStartScraperWithLock(ctx, check); err != nil {
		c.metrics.changeErrorsCounter.WithLabelValues("add").Inc()
		return err
	}

	return nil
}

func (c *Updater) handleChangeBatch(ctx context.Context, changes *sm.Changes, firstBatch bool) {
	if firstBatch && !changes.IsDeltaFirstBatch {
		c.handleFirstBatch(ctx, changes)
		return
	}

	for _, tenant := range changes.Tenants {
		c.tenantCh <- tenant
	}

	for _, checkChange := range changes.Checks {
		c.logger.Debug().Interface("check change", checkChange).Msg("got check change")

		var check model.Check
		if err := check.FromSM(checkChange.Check); err != nil {
			c.logger.Error().Err(err).Interface("check_change", checkChange).Msg("dropping check change")
			continue
		}

		switch checkChange.Operation {
		case sm.CheckOperation_CHECK_ADD:
			if err := c.handleCheckAdd(ctx, check); err != nil {
				c.metrics.changeErrorsCounter.WithLabelValues("add").Inc()
				c.logger.Error().Err(err).Msg("handling check add")
			}

		case sm.CheckOperation_CHECK_UPDATE:
			if err := c.handleCheckUpdate(ctx, check); err != nil {
				c.metrics.changeErrorsCounter.WithLabelValues("update").Inc()
				c.logger.Error().Err(err).Msg("handling check update")
			}

		case sm.CheckOperation_CHECK_DELETE:
			if err := c.handleCheckDelete(ctx, check); err != nil {
				c.metrics.changeErrorsCounter.WithLabelValues("delete").Inc()
				c.logger.Error().Err(err).Msg("handling check delete")
			}
		}
	}
}

// addAndStartScraperWithLock creates a new scraper, adds it to the list of
// scrapers managed by this updater and starts running it.
//
// This MUST be called with the scrapersMutex held.
func (c *Updater) addAndStartScraperWithLock(ctx context.Context, check model.Check) error {
	// This is a good place to filter out checks by feature flags.
	//
	// If we need to accept checks based on whether a feature flag
	// is enabled or not, we can "accept" the check from the point
	// of view of the API, and skip running it here, e.g.

	switch check.Type() {
	case sm.CheckTypeK6:
		if !c.features.IsSet(feature.K6) {
			return nil
		}

	case sm.CheckTypeMultiHttp:
		// This is correct, MultiHttp is a K6 check. We probably want
		// to abstrct this by adding a function to the settings that
		// returns whether the check requires k6 or not.
		if !c.features.IsSet(feature.K6) {
			return nil
		}

	default:
	}

	checkType := check.Type().String()

	tidStr := strconv.FormatInt(check.TenantId, 10)
	ridStr := strconv.Itoa(check.RegionId)

	scrapeCounter, err := c.metrics.scrapesCounter.GetMetricWith(prometheus.Labels{
		"type":     checkType,
		"tenantId": tidStr,
		"regionId": ridStr,
	})
	if err != nil {
		return err
	}

	scrapeErrorCounter, err := c.metrics.scrapeErrorCounter.CurryWith(prometheus.Labels{
		"type":     checkType,
		"tenantId": tidStr,
		"regionId": ridStr,
	})
	if err != nil {
		return err
	}

	scraper, err := c.scraperFactory(ctx, check, c.publisher, *c.probe, c.logger, scrapeCounter, scraper.NewIncrementerFromCounterVec(scrapeErrorCounter), c.k6Runner)
	if err != nil {
		return fmt.Errorf("cannot create new scraper: %w", err)
	}

	c.scrapers[check.GlobalID()] = scraper

	go scraper.Run(ctx)

	c.metrics.runningScrapers.WithLabelValues(checkType).Inc()

	return nil
}

// sleepCtx is like time.Sleep, but it pays attention to the
// cancellation of the provided context.
func sleepCtx(ctx context.Context, d time.Duration) error {
	var err error

	timer := time.NewTimer(d)

	select {
	case <-ctx.Done():
		err = ctx.Err()

		if !timer.Stop() {
			<-timer.C
		}

	case <-timer.C:
	}

	return err
}
