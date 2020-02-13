package main

import (
	"context"
	"log"
	"time"

	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/logproto"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/prompb"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/worldping"
	"google.golang.org/grpc"
)

type logger interface {
	Printf(format string, v ...interface{})
}

type remoteInfo struct {
	name     string
	url      string
	username string
	password string
}

type pusher struct {
	client        worldping.PusherClient
	logger        logger
	pushTimeout   time.Duration
	metricsRemote remoteInfo
	eventsRemote  remoteInfo
}

func publisher(ctx context.Context, publishCh <-chan TimeSeries, cfg config, logger *log.Logger) {
	logger.Printf("Publishing data to %s", cfg.forwarderAddress)

	conn, err := grpc.Dial(cfg.forwarderAddress, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := worldping.NewPusherClient(conn)

	pusher := pusher{
		client:      c,
		logger:      logger,
		pushTimeout: 1 * time.Second,
		metricsRemote: remoteInfo{
			name:     cfg.metrics.Name,
			url:      cfg.metrics.URL,
			username: cfg.metrics.Username,
			password: cfg.metrics.Password,
		},
		eventsRemote: remoteInfo{
			name:     cfg.events.Name,
			url:      cfg.events.URL,
			username: cfg.events.Username,
			password: cfg.events.Password,
		},
	}

	for {
		select {
		case <-ctx.Done():
			return

		case ts := <-publishCh:
			go pusher.push(ctx, ts)
		}
	}
}

func (p pusher) push(ctx context.Context, ts TimeSeries) {
	timeoutCtx, cancel := context.WithTimeout(ctx, p.pushTimeout)
	defer cancel()

	// streams := getStreams(now, endpoint, p.name)

	req := &worldping.PushRequest{
		Metrics: &worldping.MetricsRequest{
			Remote: worldping.Remote{
				Name: p.metricsRemote.name,
				Url:  p.metricsRemote.url,
				Auth: &worldping.Auth{
					Username: p.metricsRemote.username,
					Password: p.metricsRemote.password,
				},
			},
			Metrics: prompb.WriteRequest{Timeseries: ts},
		},
		Events: &worldping.EventsRequest{
			Remote: worldping.Remote{
				Name: p.eventsRemote.name,
				Url:  p.eventsRemote.url,
				Auth: &worldping.Auth{
					Username: p.eventsRemote.username,
					Password: p.eventsRemote.password,
				},
			},
			// Events: logproto.PushRequest{Streams: streams},
			Events: logproto.PushRequest{Streams: nil},
		},
	}

	resp, err := p.client.Push(timeoutCtx, req)

	p.logger.Printf("resp=%#v err=%s", resp, err)
}
