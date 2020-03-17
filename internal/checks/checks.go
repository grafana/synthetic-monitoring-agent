package checks

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/logproto"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/prompb"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/worldping"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pusher"
	"github.com/grafana/worldping-blackbox-sidecar/internal/scraper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v2"
)

type Updater struct {
	bbeConfigFilename         string
	blackboxExporterProbeURL  *url.URL
	blackboxExporterReloadURL *url.URL
	logger                    logger
	publishCh                 chan<- pusher.Payload
	apiToken                  []byte
	probe                     *worldping.Probe
	scrapersMutex             sync.Mutex
	scrapers                  map[int64]*scraper.Scraper
	conn                      *grpc.ClientConn
}

type logger interface {
	Printf(format string, v ...interface{})
}

type TimeSeries = []prompb.TimeSeries
type Streams = []logproto.Stream

func NewUpdater(conn *grpc.ClientConn, bbeConfigFilename string, blackboxExporterURL *url.URL, logger logger, publishCh chan<- pusher.Payload, apiToken []byte) (*Updater, error) {
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
		conn:                      conn,
		bbeConfigFilename:         bbeConfigFilename,
		blackboxExporterProbeURL:  blackboxExporterProbeURL,
		blackboxExporterReloadURL: blackboxExporterReloadURL,
		logger:                    logger,
		publishCh:                 publishCh,
		apiToken:                  apiToken,
		scrapers:                  make(map[int64]*scraper.Scraper),
	}, nil
}

func (c *Updater) Run(ctx context.Context) {
	for {
		// XXX(mem): add backoff? GRPC already has a backoff
		// while connecting.
		if err := c.loop(ctx); err != nil {
			c.logger.Printf("handling check changes: %s", err)
			time.Sleep(time.Second * 2)
			continue
		}

		break
	}
}

func (c *Updater) loop(ctx context.Context) error {
	c.logger.Printf("Fetching check configuration from worldping-api")

	client := worldping.NewChecksClient(c.conn)

	probeAuth := worldping.ProbeAuth{Token: c.apiToken}

	result, err := client.RegisterProbe(ctx, &probeAuth)
	if err != nil {
		return fmt.Errorf("registering probe with worldping-api: %w", err)
	} else if result.Status.Code != worldping.StatusCode_OK {
		return fmt.Errorf("registering probe with worldping-api, response: %w", result.Status.Message)
	}

	c.probe = &result.Probe

	c.logger.Printf("registered probe (%d, %s) with worldping-api", c.probe.Id, c.probe.Name)

	cc, err := client.GetChanges(ctx, &probeAuth)
	if err != nil {
		return fmt.Errorf("getting changes from worldping-api: %w", err)
	}

	for {
		select {
		case <-cc.Context().Done():
			return nil

		default:
			switch change, err := cc.Recv(); err {
			case nil:
				c.logger.Printf("Got change: %#v", change)

				switch change.Operation {
				case worldping.CheckOperation_ADD:
					if err := c.handleCheckAdd(ctx, change.Check); err != nil {
						c.logger.Printf("handling check add: %s", err)
					}

				case worldping.CheckOperation_UPDATE:
					if err := c.handleCheckUpdate(ctx, change.Check); err != nil {
						c.logger.Printf("handling check update: %s", err)
					}

				case worldping.CheckOperation_DELETE:
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
				// if the context is canceled and the
				// GRPC client processes the event
				// before we do, we get an error
				// representing the cancellation
				if status.Code(err) == codes.Canceled {
					return nil
				} else {
					return fmt.Errorf("getting changes from worldping-api: %w", err)
				}
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
	scraper, err := scraper.New(check, c.publishCh, c.probe.Name, *c.blackboxExporterProbeURL, c.logger)
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
	if err := c.updateBBEConfiguration(); err != nil {
		// XXX(mem): bail out?
		c.logger.Printf("updating blackbox-exporter configuration: %s", err)
	}

	go scraper.Run(ctx)

	return nil
}

func (c *Updater) updateBBEConfiguration() error {
	var config struct {
		Modules map[string]interface{} `yaml:"modules"`
	}

	config.Modules = make(map[string]interface{})

	for i := range c.scrapers {
		moduleName := c.scrapers[i].GetModuleName()
		config.Modules[moduleName] = c.scrapers[i].GetModuleConfig()
	}

	b, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("cannot marshal to YAML: %w", err)
	}

	fh, err := os.Create(c.bbeConfigFilename)
	if err != nil {
		return fmt.Errorf("cannot create blackbox-exporter configuration file %s: %w", c.bbeConfigFilename, err)
	}
	defer fh.Close()

	n, err := fh.Write(b)
	if err != nil {
		return fmt.Errorf("failed to write blackbox-exporter configuration file %s, wrote %d bytes: %w", c.bbeConfigFilename, n, err)
	}

	req, err := http.NewRequestWithContext(context.TODO(), "POST", c.blackboxExporterReloadURL.String(), nil)
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
