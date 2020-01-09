package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/logproto"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/prompb"
	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/worldping"
	"gonum.org/v1/gonum/stat"
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
	name          string
	client        worldping.PusherClient
	logger        logger
	pushTimeout   time.Duration
	metricsRemote remoteInfo
	eventsRemote  remoteInfo
}

func publishTestData(ctx context.Context, cfg config, logger *log.Logger) {
	logger.Printf("Publishing test data to %s", cfg.forwarderAddress)

	conn, err := grpc.Dial(cfg.forwarderAddress, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := worldping.NewPusherClient(conn)

	pusher := pusher{
		name:        "probe-1",
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

	const T = 10 * 1000 // period, ms

	ticker := time.NewTicker(T * time.Millisecond)
	defer ticker.Stop()

	logger.Printf("pushing first set")

	endpoints := make([]string, 0, 300)

	for i := 0; i < cap(endpoints); i++ {
		endpoints = append(endpoints, fmt.Sprintf("endpoint-%03d", i))
	}

	for _, endpoint := range endpoints {
		go pusher.push(ctx, endpoint)
	}

	for {
		select {
		case <-ctx.Done():
			return

		case t1 := <-ticker.C:
			logger.Printf("pushing t1=%s", t1)
			for _, endpoint := range endpoints {
				go pusher.push(ctx, endpoint)
			}
		}
	}
}

func (p pusher) push(ctx context.Context, endpoint string) {
	timeoutCtx, cancel := context.WithTimeout(ctx, p.pushTimeout)
	defer cancel()

	now := time.Now()

	timeseries := getTimeseries(now, endpoint, p.name)
	streams := getStreams(now, endpoint, p.name)

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
			Metrics: prompb.WriteRequest{Timeseries: timeseries},
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
			Events: logproto.PushRequest{Streams: streams},
		},
	}

	resp, err := p.client.Push(timeoutCtx, req)

	p.logger.Printf("resp=%#v err=%s", resp, err)
}

func makeTimeseriesLabel(name, endpoint, probe string) []*prompb.Label {
	return []*prompb.Label{
		{Name: "__name__", Value: name},
		{Name: "endpoint", Value: endpoint},
		{Name: "probe", Value: probe},
	}
}

func getTimeseries(now time.Time, endpoint, probeName string) []prompb.TimeSeries {
	const (
		N      = 20
		LossP  = 10
		base   = 100.0
		spread = 20.0
	)

	loss := rand.Intn(N/LossP + 1)
	observations := make([]float64, N-loss)

	for i := range observations {
		v := spread*rand.Float64() - spread/2.0 // [-spread/2, spread/2)
		observations[i] = base + v
	}

	sort.Float64s(observations)

	min := observations[0]
	max := observations[len(observations)-1]
	median := stat.Quantile(0.5, stat.LinInterp, observations, nil)
	mean, variance := stat.MeanVariance(observations, nil)

	return []prompb.TimeSeries{
		{
			Labels: makeTimeseriesLabel("worldping_ping_stats_loss", endpoint, probeName),
			Samples: []prompb.Sample{
				{Timestamp: now.UnixNano() / 1e6, Value: float64(loss)},
			},
		},
		{
			Labels: makeTimeseriesLabel("worldping_ping_stats_min", endpoint, probeName),
			Samples: []prompb.Sample{
				{Timestamp: now.UnixNano() / 1e6, Value: min},
			},
		},
		{
			Labels: makeTimeseriesLabel("worldping_ping_stats_max", endpoint, probeName),
			Samples: []prompb.Sample{
				{Timestamp: now.UnixNano() / 1e6, Value: max},
			},
		},
		{
			Labels: makeTimeseriesLabel("worldping_ping_stats_median", endpoint, probeName),
			Samples: []prompb.Sample{
				{Timestamp: now.UnixNano() / 1e6, Value: median},
			},
		},
		{
			Labels: makeTimeseriesLabel("worldping_ping_stats_mdev", endpoint, probeName),
			Samples: []prompb.Sample{
				{Timestamp: now.UnixNano() / 1e6, Value: math.Sqrt(variance)},
			},
		},
		{
			Labels: makeTimeseriesLabel("worldping_ping_stats_mean", endpoint, probeName),
			Samples: []prompb.Sample{
				{Timestamp: now.UnixNano() / 1e6, Value: mean},
			},
		},
	}
}

func makeStreamLabels(name, endpoint, probe, status string) string {
	return fmt.Sprintf(`{__name__="%s",endpoint="%s",probe="%s",status="%s"}`, name, endpoint, probe, status)
}

func getStreams(now time.Time, endpoint, probeName string) []*logproto.Stream {
	return []*logproto.Stream{
		{
			Labels: makeStreamLabels("worldping_ping_stats", endpoint, probeName, "OK"),
			Entries: []logproto.Entry{
				{Timestamp: now, Line: `{"message":"endpoint reachable"}`},
			},
		},
	}
}
