package k6version

import (
	"context"
	"fmt"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
)

type HandlerOpts struct {
	K6Runner k6runner.Runner
	K6Client synthetic_monitoring.K6Client
	Logger   *zerolog.Logger
}

func (ho HandlerOpts) WithDefaults() HandlerOpts {
	if ho.Logger == nil {
		nop := zerolog.Nop()
		ho.Logger = &nop
	}

	return ho
}

type Handler struct {
	HandlerOpts
	client synthetic_monitoring.K6Client
}

func NewHandler(opts HandlerOpts) (*Handler, error) {
	opts = opts.WithDefaults()

	return &Handler{HandlerOpts: opts, client: opts.K6Client}, nil
}

func (h *Handler) Handle(ctx context.Context) error {
	versionsCh := h.K6Runner.Versions(ctx)

	var sendCtx context.Context
	cancel := func() {}

	for {
		select {
		case <-ctx.Done():
			// Cancel ongoing attempt, if any.
			cancel()
			return ctx.Err()

		case versions, ok := <-versionsCh:
			h.Logger.Debug().Strs("versions", versions).Msg("Received k6 versions from runner")

			// Cancel retries for previous attempt, if any.
			cancel()

			// If the runner closed the channel, we ball.
			if !ok {
				return ctx.Err()
			}

			// Send the report asynchronously, with retries and backoff.
			sendCtx, cancel = context.WithCancel(ctx)
			go h.report(sendCtx, versions)
		}
	}
}

func (h *Handler) report(ctx context.Context, versions []string) {
	backoff := time.Second

	for {
		err := func() error {
			request := synthetic_monitoring.RegisterK6VersionRequest{
				Versions: toK6Versions(versions),
			}

			reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			resp, err := h.client.RegisterK6Version(reqCtx, &request)
			if err != nil {
				return fmt.Errorf("submitting k6 versions to API: %w", err)
			}

			if resp.Status.Code != synthetic_monitoring.StatusCode_OK {
				return fmt.Errorf("%d: %s", resp.Status.Code, resp.Status.Message)
			}

			h.Logger.Info().Strs("versions", versions).Msg("Reported k6 versions to API")

			return nil
		}()
		if err != nil {
			h.Logger.Error().Err(err).Float64("afterSeconds", backoff.Seconds()).Msg("Could not send k6 versions report, will retry")

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, time.Minute)
				continue
			}
		}

		return
	}
}

func toK6Versions(versions []string) []synthetic_monitoring.K6Version {
	k6Versions := make([]synthetic_monitoring.K6Version, 0, len(versions))

	for _, v := range versions {
		k6Versions = append(k6Versions, synthetic_monitoring.K6Version{Version: v})
	}

	return k6Versions
}
