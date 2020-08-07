package checks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/grafana/loki/pkg/logproto"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/scraper"
	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	errNotAuthorized    = errors.New("probe not authorized")
	errTransportClosing = errors.New("transport closing")
)

// Updater represents a probe along with the collection of scrapers
// running on that probe and it manages the configuration for
// blackbox-exporter that corresponds to the collection of scrapers.
type Updater struct {
	api           apiInfo
	logger        zerolog.Logger
	publishCh     chan<- pusher.Payload
	probe         *sm.Probe
	scrapersMutex sync.Mutex
	scrapers      map[int64]*scraper.Scraper
	metrics       metrics
}

type apiInfo struct {
	conn *grpc.ClientConn
}

type metrics struct {
	changesCounter      *prometheus.CounterVec
	changeErrorsCounter *prometheus.CounterVec
	runningScrapers     *prometheus.GaugeVec
	scrapesCounter      *prometheus.CounterVec
	scrapeErrorCounter  *prometheus.CounterVec
	probeInfo           *prometheus.GaugeVec
}

type TimeSeries = []prompb.TimeSeries
type Streams = []logproto.Stream

func NewUpdater(conn *grpc.ClientConn, logger zerolog.Logger, publishCh chan<- pusher.Payload, promRegisterer prometheus.Registerer) (*Updater, error) {

	changesCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sm_agent",
		Subsystem: "updater",
		Name:      "changes_total",
		Help:      "Total number of changes processed.",
	}, []string{
		"type",
	})

	if err := promRegisterer.Register(changesCounter); err != nil {
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

	if err := promRegisterer.Register(changeErrorsCounter); err != nil {
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

	if err := promRegisterer.Register(runningScrapers); err != nil {
		return nil, err
	}

	scrapesCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sm_agent",
		Subsystem: "scraper",
		Name:      "operations_total",
		Help:      "Total number of scrape operations performed.",
	}, []string{
		"check_id",
		"probe",
	})

	if err := promRegisterer.Register(scrapesCounter); err != nil {
		return nil, err
	}

	scrapeErrorCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sm_agent",
		Subsystem: "scraper",
		Name:      "errors_total",
		Help:      "Total number of scraper errors.",
	}, []string{
		"check_id",
		"probe",
		"type",
	})

	if err := promRegisterer.Register(scrapeErrorCounter); err != nil {
		return nil, err
	}

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

	if err := promRegisterer.Register(probeInfoGauge); err != nil {
		return nil, err
	}

	return &Updater{
		api: apiInfo{
			conn: conn,
		},
		logger:    logger,
		publishCh: publishCh,
		scrapers:  make(map[int64]*scraper.Scraper),
		metrics: metrics{
			changesCounter:      changesCounter,
			changeErrorsCounter: changeErrorsCounter,
			runningScrapers:     runningScrapers,
			scrapesCounter:      scrapesCounter,
			scrapeErrorCounter:  scrapeErrorCounter,
			probeInfo:           probeInfoGauge,
		},
	}, nil
}

func (c *Updater) Run(ctx context.Context) error {
	for {
		err := c.loop(ctx)
		switch {
		case err == nil:
			return nil

		case errors.Is(err, errNotAuthorized):
			// our token is invalid, bail out?
			c.logger.Error().
				Err(err).
				Str("connection_state", c.api.conn.GetState().String()).
				Msg("cannot connect, bailing out")
			return err

		case errors.Is(err, errTransportClosing):
			// the other end went away? Allow GRPC to reconnect.
			c.logger.Warn().
				Err(err).
				Str("connection_state", c.api.conn.GetState().String()).
				Msg("the other end closed the connection, trying to reconnect")
			continue

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
			time.Sleep(2 * time.Second)
			continue
		}

	}
}

func (c *Updater) loop(ctx context.Context) error {
	c.logger.Info().Msg("fetching check configuration from synthetic-monitoring-api")

	client := sm.NewChecksClient(c.api.conn)

	grpcErrorHandler := func(action string, err error) error {
		status, ok := status.FromError(err)
		c.logger.Error().Err(err).Uint32("code", uint32(status.Code())).Msg(status.Message())

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

		default:
			return status.Err()
		}
	}

	result, err := client.RegisterProbe(ctx, &sm.Void{})
	if err != nil {
		return grpcErrorHandler("registering probe with synthetic-monitoring-api", err)
	}

	switch result.Status.Code {
	case sm.StatusCode_OK:
		// continue

	case sm.StatusCode_NOT_AUTHORIZED:
		return errNotAuthorized

	default:
		return fmt.Errorf("registering probe with synthetic-monitoring-api, response: %s", result.Status.Message)
	}

	c.probe = &result.Probe

	c.logger.Info().Int64("probe id", c.probe.Id).Str("probe name", c.probe.Name).Msg("registered probe with synthetic-monitoring-api")

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

	cc, err := client.GetChanges(ctx, &sm.Void{})
	if err != nil {
		return grpcErrorHandler("requesting changes from synthetic-monitoring-api", err)
	}

	if err := c.processChanges(ctx, cc); err != nil {
		return grpcErrorHandler("getting changes from synthetic-monitoring-api", err)
	}

	return nil
}

func (c *Updater) processChanges(ctx context.Context, cc sm.Checks_GetChangesClient) error {
	firstBatchDone := false

	for {
		select {
		case <-cc.Context().Done():
			return nil

		default:
			switch msg, err := cc.Recv(); err {
			case nil:
				if firstBatchDone {
					c.handleChangeBatch(ctx, msg.Changes)
				} else {
					c.handleFirstBatch(ctx, msg.Changes)
					firstBatchDone = true
				}

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

func (c *Updater) handleCheckAdd(ctx context.Context, check sm.Check) error {
	c.metrics.changesCounter.WithLabelValues("add").Inc()

	if err := check.Validate(); err != nil {
		return fmt.Errorf("invalid check: %w", err)
	}

	c.scrapersMutex.Lock()
	defer c.scrapersMutex.Unlock()

	if running, found := c.scrapers[check.Id]; found {
		// we can get here if the API sent us a check add twice:
		// once during the initial connection and another right
		// after that. The window for that is small, but it
		// exists.

		return fmt.Errorf("check with id %d already exists (version %s)", check.Id, running.ConfigVersion())
	}

	return c.addAndStartScraperWithLock(ctx, check)
}

func (c *Updater) handleCheckUpdate(ctx context.Context, check sm.Check) error {
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
func (c *Updater) handleCheckUpdateWithLock(ctx context.Context, check sm.Check) error {
	scraper, found := c.scrapers[check.Id]
	if !found {
		c.logger.Warn().Int64("check_id", check.Id).Msg("update request for an unknown check")
		return c.addAndStartScraperWithLock(ctx, check)
	}

	// this is the lazy way to update the scraper: tear everything
	// down, start it again.

	scraper.Stop()
	checkType := scraper.CheckType()
	delete(c.scrapers, check.Id)

	c.metrics.runningScrapers.WithLabelValues(checkType).Dec()

	return c.addAndStartScraperWithLock(ctx, check)
}

func (c *Updater) handleCheckDelete(ctx context.Context, check sm.Check) error {
	c.metrics.changesCounter.WithLabelValues("delete").Inc()

	c.scrapersMutex.Lock()
	defer c.scrapersMutex.Unlock()

	scraper, found := c.scrapers[check.Id]
	if !found {
		c.logger.Warn().Int64("check_id", check.Id).Msg("delete request for an unknown check")
		return nil
	}

	scraper.Stop()
	checkType := scraper.CheckType()

	delete(c.scrapers, check.Id)

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
// didn't send, becuase that means it didn't know we have those (they got
// deleted during the reconnect, and the server didn't send them).
//
// We have to do this exactly once per reconnect. It's up to the calling code
// to ensure this.
func (c *Updater) handleFirstBatch(ctx context.Context, changes []sm.CheckChange) {
	newChecks := make(map[int64]struct{})

	c.scrapersMutex.Lock()
	defer c.scrapersMutex.Unlock()

	// add checks from the provided list
	for _, change := range changes {
		c.logger.Debug().Interface("change", change).Msg("got change")

		switch change.Operation {
		case sm.CheckOperation_CHECK_ADD:
			if err := c.handleInitialChangeAddWithLock(ctx, change.Check); err != nil {
				c.metrics.changeErrorsCounter.WithLabelValues("add").Inc()
				c.logger.Error().
					Err(err).
					Int64("check_id", change.Check.Id).
					Msg("adding check failed, dropping check")
				continue
			}

			// add this to the list of checks we have seen during
			// this operation
			newChecks[change.Check.Id] = struct{}{}

		default:
			// we should never hit this because the first time we
			// connect the server will only send adds.
			c.logger.Warn().
				Interface("check", change.Check).
				Str("operation", change.Operation.String()).
				Msg("unexpected operation, dropping change")
			continue
		}
	}

	// remove all the running scrapers that weren't sent with the first batch
	for id, scraper := range c.scrapers {
		if _, found := newChecks[id]; found {
			continue
		}

		checkType := scraper.CheckType()
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
func (c *Updater) handleInitialChangeAddWithLock(ctx context.Context, check sm.Check) error {
	if running, found := c.scrapers[check.Id]; found {
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

func (c *Updater) handleChangeBatch(ctx context.Context, changes []sm.CheckChange) {
	for _, change := range changes {
		c.logger.Debug().Interface("change", change).Msg("got change")

		switch change.Operation {
		case sm.CheckOperation_CHECK_ADD:
			if err := c.handleCheckAdd(ctx, change.Check); err != nil {
				c.metrics.changeErrorsCounter.WithLabelValues("add").Inc()
				c.logger.Error().Err(err).Msg("handling check add")
			}

		case sm.CheckOperation_CHECK_UPDATE:
			if err := c.handleCheckUpdate(ctx, change.Check); err != nil {
				c.metrics.changeErrorsCounter.WithLabelValues("update").Inc()
				c.logger.Error().Err(err).Msg("handling check update")
			}

		case sm.CheckOperation_CHECK_DELETE:
			if err := c.handleCheckDelete(ctx, change.Check); err != nil {
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
func (c *Updater) addAndStartScraperWithLock(ctx context.Context, check sm.Check) error {
	scrapeCounter := c.metrics.scrapesCounter.With(prometheus.Labels{
		"check_id": strconv.FormatInt(check.Id, 10),
		"probe":    c.probe.Name,
	})

	scrapeErrorCounter, err := c.metrics.scrapeErrorCounter.CurryWith(prometheus.Labels{
		"check_id": strconv.FormatInt(check.Id, 10),
		"probe":    c.probe.Name,
	})
	if err != nil {
		return err
	}

	scraper, err := scraper.New(ctx, check, c.publishCh, *c.probe, c.logger, scrapeCounter, scrapeErrorCounter)
	if err != nil {
		return fmt.Errorf("cannot create new scraper: %w", err)
	}

	c.scrapers[check.Id] = scraper

	go scraper.Run(ctx)

	c.metrics.runningScrapers.WithLabelValues(scraper.CheckType()).Inc()

	return nil
}
