package main

import (
	"context"
	"fmt"
	"io"

	protobuf "github.com/gogo/protobuf/types"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/worldping"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type checksUpdater struct {
	apiServerAddr string
	logger        logger
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

	return nil
}
