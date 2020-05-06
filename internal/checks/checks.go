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
	"sync"
	"time"

	"github.com/grafana/loki/pkg/logproto"
	"github.com/grafana/worldping-api/pkg/pb/worldping"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pusher"
	"github.com/grafana/worldping-blackbox-sidecar/internal/scraper"
	"github.com/prometheus/prometheus/prompb"
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
	bbeInfo       bbeInfo
	api           apiInfo
	logger        logger
	publishCh     chan<- pusher.Payload
	probe         *worldping.Probe
	scrapersMutex sync.Mutex
	scrapers      map[int64]*scraper.Scraper
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

type logger interface {
	Printf(format string, v ...interface{})
}

type TimeSeries = []prompb.TimeSeries
type Streams = []logproto.Stream

func NewUpdater(conn *grpc.ClientConn, bbeConfigFilename string, blackboxExporterURL *url.URL, logger logger, publishCh chan<- pusher.Payload) (*Updater, error) {
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

	return &Updater{
		api: apiInfo{
			conn: conn,
		},
		bbeInfo: bbeInfo{
			configFilename: bbeConfigFilename,
			probeURL:       blackboxExporterProbeURL,
			reloadURL:      blackboxExporterReloadURL.String(),
		},
		logger:    logger,
		publishCh: publishCh,
		scrapers:  make(map[int64]*scraper.Scraper),
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
			return err

		case errors.Is(err, errTransportClosing):
			// the other end went away? Since we don't know if it's coming back, bail out.
			//
			// TODO(mem): we could try to rebuild the connection (fall to the default case below), but that
			// requires some limit to avoid spining forever
			return err

		case errors.Is(err, context.Canceled):
			// context was cancelled, clean up
			return nil

		default:
			// TODO(mem): this might be a transient error (e.g. bad connection). We need to add a backoff
			// here.
			c.logger.Printf("while handling check changes: %s", err)
			time.Sleep(time.Second * 2)
			continue
		}

	}
}

func (c *Updater) loop(ctx context.Context) error {
	c.logger.Printf("Fetching check configuration from worldping-api")

	client := worldping.NewChecksClient(c.api.conn)

	grpcErrorHandler := func(action string, err error) error {
		status, ok := status.FromError(err)
		c.logger.Printf("updater error: %#v message: %q code: %d", err, status.Message(), status.Code())

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

	c.logger.Printf("registered probe (%d, %s) with worldping-api", c.probe.Id, c.probe.Name)

	cc, err := client.GetChanges(ctx, &worldping.Void{})
	if err != nil {
		return grpcErrorHandler("requesting changes from worldping-api", err)
	}

	// XXX(mem): possibly create a new context here that gets
	// cancelled when this function returns, so that the scrapers
	// are stopped; they should also be removed from the map of
	// running scrapers.

	for {
		select {
		case <-cc.Context().Done():
			return nil

		default:
			switch change, err := cc.Recv(); err {
			case nil:
				c.logger.Printf("Got change: %#v", change)

				switch change.Operation {
				case worldping.CheckOperation_CHECK_ADD:
					if err := c.handleCheckAdd(ctx, change.Check); err != nil {
						c.logger.Printf("handling check add: %s", err)
					}

				case worldping.CheckOperation_CHECK_UPDATE:
					if err := c.handleCheckUpdate(ctx, change.Check); err != nil {
						c.logger.Printf("handling check update: %s", err)
					}

				case worldping.CheckOperation_CHECK_DELETE:
					if err := c.handleCheckDelete(ctx, change.Check); err != nil {
						c.logger.Printf("handling check delete: %s", err)
					}
				}

			case io.EOF:
				c.logger.Printf("No more messages?")
				// XXX(mem): what happened here? The
				// other end told us there are no more
				// changes. Stop? Is it restarting?
				return nil

			default:
				return grpcErrorHandler("getting changes from worldping-api", err)
			}
		}
	}
}

func (c *Updater) handleCheckAdd(ctx context.Context, check worldping.Check) error {
	c.scrapersMutex.Lock()
	defer c.scrapersMutex.Unlock()

	if _, found := c.scrapers[check.Id]; found {
		// we can get here if the API sent us a check add twice:
		// once during the initial conneciton and another right
		// after that. The window for that is small, but it
		// exists.
		return fmt.Errorf("check with id %d already exists", check.Id)
	}

	return c.addAndStartScraper(ctx, check)
}

func (c *Updater) handleCheckUpdate(ctx context.Context, check worldping.Check) error {
	c.scrapersMutex.Lock()
	defer c.scrapersMutex.Unlock()

	scraper, found := c.scrapers[check.Id]
	if !found {
		c.logger.Printf("got an update request for an unknown check: %#v", check)
		return nil
	}

	// this is the lazy way to update the scraper: tear everything
	// down, start it again.

	scraper.Stop()
	delete(c.scrapers, check.Id)

	return c.addAndStartScraper(ctx, check)
}

func (c *Updater) handleCheckDelete(ctx context.Context, check worldping.Check) error {
	c.scrapersMutex.Lock()
	defer c.scrapersMutex.Unlock()

	scraper, found := c.scrapers[check.Id]
	if !found {
		c.logger.Printf("got a delete request for an unknown check: %#v", check)
		return nil
	}

	scraper.Stop()

	delete(c.scrapers, check.Id)

	return nil
}

// addAndStartScraper creates a new scraper, adds it to the list of
// scrapers managed by this updater and starts running it.
//
// This MUST be called with the scrapersMutex held.
func (c *Updater) addAndStartScraper(ctx context.Context, check worldping.Check) error {
	scraper, err := scraper.New(check, c.publishCh, *c.probe, *c.bbeInfo.probeURL, c.logger)
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
		c.logger.Printf("updating blackbox-exporter configuration: %s", err)
	}

	go scraper.Run(ctx)

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
