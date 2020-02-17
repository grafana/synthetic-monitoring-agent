package main

import (
	"context"
	"fmt"
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

type publisher struct {
	publishCh <-chan TimeSeries
	cfg       config
	logger    logger
}

func (p publisher) run(ctx context.Context) error {
	p.logger.Printf("Publishing data to %s", p.cfg.forwarderAddress)

	conn, err := grpc.Dial(p.cfg.forwarderAddress, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", p.cfg.forwarderAddress, err)
	}
	defer conn.Close()

	c := worldping.NewPusherClient(conn)

	pusher := pusher{
		client:      c,
		logger:      p.logger,
		pushTimeout: 1 * time.Second,
		metricsRemote: remoteInfo{
			name:     p.cfg.metrics.Name,
			url:      p.cfg.metrics.URL,
			username: p.cfg.metrics.Username,
			password: p.cfg.metrics.Password,
		},
		eventsRemote: remoteInfo{
			name:     p.cfg.events.Name,
			url:      p.cfg.events.URL,
			username: p.cfg.events.Username,
			password: p.cfg.events.Password,
		},
	}

	for {
		select {
		case <-ctx.Done():
			return nil

		case ts := <-p.publishCh:
			go pusher.push(ctx, ts)
		}
	}

	return nil
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
