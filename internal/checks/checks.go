package checks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/grafana/loki/pkg/logproto"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pusher"
	"github.com/grafana/worldping-blackbox-sidecar/internal/scraper"
	"github.com/grafana/worldping-blackbox-sidecar/pkg/pb/worldping"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v2"
)

var (
	errNotAuthorized    = errors.New("probe not authorized")
	errTransportClosing = errors.New("transport closing")
)

// Updater represents a probe along with the collection of scrapers
// running on that probe and it manages the configuration for
// blackbox-exporter that corresponds to the collection of scrapers.
type Updater struct {
	bbeInfo             bbeInfo
	api                 apiInfo
	logger              zerolog.Logger
	publishCh           chan<- pusher.Payload
	probe               *worldping.Probe
	scrapersMutex       sync.Mutex
	scrapers            map[int64]*scraper.Scraper
	changesCounter      *prometheus.CounterVec
	changeErrorsCounter *prometheus.CounterVec
	runningScrapers     *prometheus.GaugeVec
	scrapesCounter      *prometheus.CounterVec
	scrapeErrorCounter  *prometheus.CounterVec
}

// bbeInfo represents the information necessary to communicate with
// blackbox-exporter
type bbeInfo struct {
	configFilename string
	reloadURL      string
	probeURL       *url.URL
}

type apiInfo struct {
	conn *grpc.ClientConn
}

type TimeSeries = []prompb.TimeSeries
type Streams = []logproto.Stream

func NewUpdater(conn *grpc.ClientConn, bbeConfigFilename string, blackboxExporterURL *url.URL, logger zerolog.Logger, publishCh chan<- pusher.Payload, promRegisterer prometheus.Registerer) (*Updater, error) {
	if blackboxExporterURL == nil {
		return nil, fmt.Errorf("invalid blackbox-exporter URL")
	}

	blackboxExporterProbeURL, err := blackboxExporterURL.Parse("probe")
	if err != nil {
		return nil, err
	}

	blackboxExporterReloadURL, err := blackboxExporterURL.Parse("-/reload")
	if err != nil {
		return nil, err
	}

	changesCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "worldping_bbe_sidecar",
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
		Namespace: "worldping_bbe_sidecar",
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
		Namespace: "worldping_bbe_sidecar",
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
		Namespace: "worldping_bbe_sidecar",
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
		Namespace: "worldping_bbe_sidecar",
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

	return &Updater{
		api: apiInfo{
			conn: conn,
		},
		bbeInfo: bbeInfo{
			configFilename: bbeConfigFilename,
			probeURL:       blackboxExporterProbeURL,
			reloadURL:      blackboxExporterReloadURL.String(),
		},
		logger:              logger,
		publishCh:           publishCh,
		scrapers:            make(map[int64]*scraper.Scraper),
		changesCounter:      changesCounter,
		changeErrorsCounter: changeErrorsCounter,
		runningScrapers:     runningScrapers,
		scrapesCounter:      scrapesCounter,
		scrapeErrorCounter:  scrapeErrorCounter,
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
	c.logger.Info().Msg("fetching check configuration from worldping-api")

	client := worldping.NewChecksClient(c.api.conn)

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

	result, err := client.RegisterProbe(ctx, &worldping.Void{})
	if err != nil {
		return grpcErrorHandler("registering probe with worldping-api", err)
	}

	switch result.Status.Code {
	case worldping.StatusCode_OK:
		// continue

	case worldping.StatusCode_NOT_AUTHORIZED:
		return errNotAuthorized

	default:
		return fmt.Errorf("registering probe with worldping-api, response: %s", result.Status.Message)
	}

	c.probe = &result.Probe

	c.logger.Info().Int64("probe id", c.probe.Id).Str("probe name", c.probe.Name).Msg("registered probe with worldping-api")

	cc, err := client.GetChanges(ctx, &worldping.Void{})
	if err != nil {
		return grpcErrorHandler("requesting changes from worldping-api", err)
	}

	if err := c.processChanges(ctx, cc); err != nil {
		return grpcErrorHandler("getting changes from worldping-api", err)
	}

	return nil
}

func (c *Updater) processChanges(ctx context.Context, cc worldping.Checks_GetChangesClient) error {
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

func (c *Updater) handleCheckAdd(ctx context.Context, check worldping.Check) error {
	c.changesCounter.WithLabelValues("add").Inc()

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

func (c *Updater) handleCheckUpdate(ctx context.Context, check worldping.Check) error {
	c.changesCounter.WithLabelValues("update").Inc()

	if err := check.Validate(); err != nil {
		return fmt.Errorf("invalid check: %w", err)
	}

	c.scrapersMutex.Lock()
	defer c.scrapersMutex.Unlock()

	return c.handleCheckUpdateWithLock(ctx, check)
}

// handleCheckUpdateWithLock is the bottom half of handleCheckUpdate. It
// MUST be called with the scrapersMutex lock held.
func (c *Updater) handleCheckUpdateWithLock(ctx context.Context, check worldping.Check) error {
	scraper, found := c.scrapers[check.Id]
	if !found {
		c.logger.Warn().Int64("check_id", check.Id).Msg("update request for an unknown check")
		return nil
	}

	// this is the lazy way to update the scraper: tear everything
	// down, start it again.

	scraper.Stop()
	checkType := scraper.CheckType()
	delete(c.scrapers, check.Id)

	c.runningScrapers.WithLabelValues(checkType).Dec()

	return c.addAndStartScraperWithLock(ctx, check)
}

func (c *Updater) handleCheckDelete(ctx context.Context, check worldping.Check) error {
	c.changesCounter.WithLabelValues("delete").Inc()

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

	c.runningScrapers.WithLabelValues(checkType).Dec()

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
func (c *Updater) handleFirstBatch(ctx context.Context, changes []worldping.CheckChange) {
	newChecks := make(map[int64]struct{})

	c.scrapersMutex.Lock()
	defer c.scrapersMutex.Unlock()

	// add checks from the provided list
	for _, change := range changes {
		c.logger.Debug().Interface("change", change).Msg("got change")

		switch change.Operation {
		case worldping.CheckOperation_CHECK_ADD:
			if err := c.handleInitialChangeAddWithLock(ctx, change.Check); err != nil {
				c.changeErrorsCounter.WithLabelValues("add").Inc()
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

		c.runningScrapers.WithLabelValues(checkType).Dec()
	}
}

// handleCheckUpdateWithLock the specified check to the running checks.
//
// It deals with the case where this check is the product of a reconnection
// and changes the operation to an update if necessary.
//
// This function MUST be called with the scrapers mutex held.
func (c *Updater) handleInitialChangeAddWithLock(ctx context.Context, check worldping.Check) error {
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

	c.changesCounter.WithLabelValues("add").Inc()

	if err := check.Validate(); err != nil {
		return err
	}

	if err := c.addAndStartScraperWithLock(ctx, check); err != nil {
		c.changeErrorsCounter.WithLabelValues("add").Inc()
		return err
	}

	return nil
}

func (c *Updater) handleChangeBatch(ctx context.Context, changes []worldping.CheckChange) {
	for _, change := range changes {
		c.logger.Debug().Interface("change", change).Msg("got change")

		switch change.Operation {
		case worldping.CheckOperation_CHECK_ADD:
			if err := c.handleCheckAdd(ctx, change.Check); err != nil {
				c.changeErrorsCounter.WithLabelValues("add").Inc()
				c.logger.Error().Err(err).Msg("handling check add")
			}

		case worldping.CheckOperation_CHECK_UPDATE:
			if err := c.handleCheckUpdate(ctx, change.Check); err != nil {
				c.changeErrorsCounter.WithLabelValues("update").Inc()
				c.logger.Error().Err(err).Msg("handling check update")
			}

		case worldping.CheckOperation_CHECK_DELETE:
			if err := c.handleCheckDelete(ctx, change.Check); err != nil {
				c.changeErrorsCounter.WithLabelValues("delete").Inc()
				c.logger.Error().Err(err).Msg("handling check delete")
			}
		}
	}
}

// addAndStartScraperWithLock creates a new scraper, adds it to the list of
// scrapers managed by this updater and starts running it.
//
// This MUST be called with the scrapersMutex held.
func (c *Updater) addAndStartScraperWithLock(ctx context.Context, check worldping.Check) error {
	scrapeCounter := c.scrapesCounter.With(prometheus.Labels{
		"check_id": strconv.FormatInt(check.Id, 10),
		"probe":    c.probe.Name,
	})

	scrapeErrorCounter, err := c.scrapeErrorCounter.CurryWith(prometheus.Labels{
		"check_id": strconv.FormatInt(check.Id, 10),
		"probe":    c.probe.Name,
	})
	if err != nil {
		return err
	}

	scraper, err := scraper.New(ctx, check, c.publishCh, *c.probe, *c.bbeInfo.probeURL, c.logger, scrapeCounter, scrapeErrorCounter)
	if err != nil {
		return fmt.Errorf("cannot create new scraper: %w", err)
	}

	c.scrapers[check.Id] = scraper

	// XXX(mem): this needs to be rate-limited somehow, it doesn't
	// make sense to post 600 reload requests in a second. The trick
	// is that the scraper cannot run until the configuration has
	// been updated.
	//
	// Possibly delay starting the scraper using a list and a
	// timeout?
	if err := c.bbeInfo.updateConfig(c.scrapers); err != nil {
		// XXX(mem): bail out?
		c.logger.Error().Err(err).Int64("check_id", check.Id).Msg("updating blackbox-exporter configuration")
	}

	go scraper.Run(ctx)

	c.runningScrapers.WithLabelValues(scraper.CheckType()).Inc()

	return nil
}

func (bbe *bbeInfo) updateConfig(scrapers map[int64]*scraper.Scraper) error {
	var config struct {
		Modules map[string]interface{} `yaml:"modules"`
	}

	config.Modules = make(map[string]interface{})

	for _, scraper := range scrapers {
		moduleName := scraper.GetModuleName()
		config.Modules[moduleName] = scraper.GetModuleConfig()
	}

	b, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("cannot marshal to YAML: %w", err)
	}

	fh, err := os.Create(bbe.configFilename)
	if err != nil {
		return fmt.Errorf("cannot create blackbox-exporter configuration file %s: %w", bbe.configFilename, err)
	}
	defer fh.Close()

	n, err := fh.Write(b)
	if err != nil {
		return fmt.Errorf("failed to write blackbox-exporter configuration file %s, wrote %d bytes: %w", bbe.configFilename, n, err)
	}

	req, err := http.NewRequestWithContext(context.TODO(), "POST", bbe.reloadURL, nil)
	if err != nil {
		return fmt.Errorf("creating new request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("posting configuration reload request : %w", err)
	}

	defer func() {
		// drain body
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response posting configuration reload request : %s", resp.Status)
	}

	return nil
}
