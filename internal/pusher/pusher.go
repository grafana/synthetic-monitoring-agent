package pusher

import (
	"context"
	"fmt"
	"time"

	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/logproto"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/prompb"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/worldping"
	"google.golang.org/grpc"
)

type Config struct {
	ForwarderAddress string

	Metrics struct {
		Name     string `required:"true"`
		URL      string `required:"true"`
		Username string `required:"true"`
		Password string `required:"true"`
	}

	Events struct {
		Name     string `required:"true"`
		URL      string `required:"true"`
		Username string `required:"true"`
		Password string `required:"true"`
	}
}

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

type Publisher struct {
	publishCh <-chan Payload
	cfg       Config
	logger    logger
}

func NewPublisher(publishCh <-chan Payload, config Config, logger logger) *Publisher {
	return &Publisher{
		publishCh: publishCh,
		cfg:       config,
		logger:    logger,
	}
}

func (p Publisher) Run(ctx context.Context) error {
	p.logger.Printf("Publishing data to %s", p.cfg.ForwarderAddress)

	conn, err := grpc.Dial(p.cfg.ForwarderAddress, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", p.cfg.ForwarderAddress, err)
	}
	defer conn.Close()

	c := worldping.NewPusherClient(conn)

	pusher := pusher{
		client:      c,
		logger:      p.logger,
		pushTimeout: 1 * time.Second,
		metricsRemote: remoteInfo{
			name:     p.cfg.Metrics.Name,
			url:      p.cfg.Metrics.URL,
			username: p.cfg.Metrics.Username,
			password: p.cfg.Metrics.Password,
		},
		eventsRemote: remoteInfo{
			name:     p.cfg.Events.Name,
			url:      p.cfg.Events.URL,
			username: p.cfg.Events.Username,
			password: p.cfg.Events.Password,
		},
	}

	for {
		select {
		case <-ctx.Done():
			return nil

		case payload := <-p.publishCh:
			go pusher.push(ctx, payload)
		}
	}
}

type Payload interface {
	Metrics() []prompb.TimeSeries
	Streams() []logproto.Stream
}

func (p pusher) push(ctx context.Context, payload Payload) {
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
			Metrics: prompb.WriteRequest{Timeseries: payload.Metrics()},
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
			Events: logproto.PushRequest{Streams: payload.Streams()},
		},
	}

	resp, err := p.client.Push(timeoutCtx, req)

	if err != nil {
		p.logger.Printf("resp=%#v err=%s", resp, err)
	}
}
