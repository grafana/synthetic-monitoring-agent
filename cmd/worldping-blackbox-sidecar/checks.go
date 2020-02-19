package main

import (
	"context"
	"fmt"
	"io"
	"net/url"

	protobuf "github.com/gogo/protobuf/types"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/worldping"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type checksUpdater struct {
	apiServerAddr       string
	blackboxExporterURL *url.URL
	logger              logger
	publishCh           chan<- TimeSeries
	probeName           string
}

func (c checksUpdater) run(ctx context.Context) {
	for {
		// XXX(mem): add backoff? GRPC already has a backoff
		// while connecting.
		if err := c.loop(ctx); err != nil {
			c.logger.Printf("handling check changes: %s", err)
			continue
		}

		break
	}
}

func (c checksUpdater) loop(ctx context.Context) error {
	c.logger.Printf("Fetching check configuration from %s", c.apiServerAddr)

	conn, err := grpc.Dial(c.apiServerAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("dialing GRPC server %s: %w", c.apiServerAddr, err)
	}
	defer conn.Close()

	client := worldping.NewChecksClient(conn)

	var empty protobuf.Empty
	cc, err := client.GetChanges(ctx, &empty)
	if err != nil {
		return fmt.Errorf("getting changes from %s: %w", c.apiServerAddr, err)
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

				case worldping.CheckOperation_DELETE:
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
					return fmt.Errorf("getting changes from %s: %w", c.apiServerAddr, err)
				}
			}
		}
	}
}

func (c checksUpdater) handleCheckAdd(ctx context.Context, check worldping.Check) error {
	var (
		module string
		target string
	)

	// Map the change to a blackbox exporter module
	if check.Settings.PingSettings != nil {
		module = "icmp_v4"
		target = check.Settings.PingSettings.Hostname
	} else if check.Settings.HttpSettings != nil {
		module = "http_2xx_v4"
		target = check.Settings.HttpSettings.Url
	} else if check.Settings.DnsSettings != nil {
		module = "dns_v4"
		target = check.Settings.DnsSettings.Name
	} else {
		return fmt.Errorf("unsupported change")
	}

	if c.blackboxExporterURL == nil {
		c.logger.Printf("no blackbox exporter URL configured, ignoring check change")
		return nil
	}

	u := *c.blackboxExporterURL
	q := u.Query()
	q.Add("target", target)
	q.Add("module", module)
	u.RawQuery = q.Encode()

	scraper := scraper{
		publishCh: c.publishCh,
		probeName: c.probeName,
		target:    u.String(),
		endpoint:  target,
		logger:    c.logger,
	}

	// XXX(mem): this needs to change to check for existing queries
	// and to handle enabling / disabling of checks
	go scraper.run(ctx)

	return nil
}
