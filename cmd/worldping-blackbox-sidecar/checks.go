package main

import (
	"context"
	"io"
	"log"

	protobuf "github.com/gogo/protobuf/types"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/worldping"
	"google.golang.org/grpc"
)

func checksUpdater(ctx context.Context, apiServerAddr string, logger *log.Logger) {
	logger.Printf("Fetching check configuration from %s", apiServerAddr)

	conn, err := grpc.Dial(apiServerAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := worldping.NewChecksClient(conn)

	var empty protobuf.Empty
	cc, err := c.GetChanges(ctx, &empty)
	if err != nil {
		logger.Fatalf("cannot get changes: %v", err)
	}

	for {
		select {
		case <-cc.Context().Done():
			return

		default:
			switch change, err := cc.Recv(); err {
			case nil:
				logger.Printf("Got change: %#v", change)

			case io.EOF:
				logger.Printf("No more messages?")

			default:
				logger.Printf("Error while getting changes: %s", err)
			}
		}

	}
}
