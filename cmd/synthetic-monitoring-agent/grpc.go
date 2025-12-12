package main

import (
	"context"
	"crypto/tls"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// interceptorLogger adapts zerolog.Logger to the grpc-middleware logging interface.
// This allows structured logging of all gRPC client calls with proper log levels.
func interceptorLogger(l zerolog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l := l.With().Fields(fields).Logger()

		switch lvl {
		case logging.LevelDebug:
			l.Debug().Msg(msg)
		case logging.LevelInfo:
			l.Info().Msg(msg)
		case logging.LevelWarn:
			l.Warn().Msg(msg)
		case logging.LevelError:
			l.Error().Msg(msg)
		default:
			l.Info().Msg(msg)
		}
	})
}

func dialAPIServer(addr string, allowInsecure bool, apiToken string, logger zerolog.Logger, grpcClientMetrics *grpcprom.ClientMetrics) (*grpc.ClientConn, error) {
	apiCreds := creds{
		Token:         apiToken,
		AllowInsecure: allowInsecure,
	}

	// Configure logging options for gRPC interceptor
	logOpts := []logging.Option{
		// Don't include payload to avoid logging sensitive data by default.
		logging.WithLogOnEvents(
			logging.StartCall,
			logging.FinishCall,
		),

		logging.WithFieldsFromContext(func(ctx context.Context) logging.Fields {
			return nil
		}),

		logging.WithDurationField(func(duration time.Duration) logging.Fields {
			const (
				precision = 6
				bits      = 64
			)

			return logging.Fields{
				"grpc.duration",
				strconv.FormatFloat(duration.Seconds(), 'g', precision, bits),
			}
		}),
	}

	opts := []grpc.DialOption{
		grpc.WithPerRPCCredentials(apiCreds),
		// Enable structured logging for all gRPC calls.
		// Logs start and finish of calls with method, duration, and status code.
		grpc.WithChainUnaryInterceptor(
			logging.UnaryClientInterceptor(interceptorLogger(logger), logOpts...),
			grpcClientMetrics.UnaryClientInterceptor(),
		),
		grpc.WithChainStreamInterceptor(
			logging.StreamClientInterceptor(interceptorLogger(logger), logOpts...),
			grpcClientMetrics.StreamClientInterceptor(),
		),
		// Keep-alive is necessary to detect network failures in absence of writes from the client.
		// Without it, the agent would hang if the server disappears while waiting for a response.
		// See https://github.com/grpc/grpc/blob/master/doc/keepalive.md
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			// Time: Send pings every 90s to detect network failures faster than TCP timeout.
			// This ensures the agent knows when the API server is unreachable and can reconnect.
			// 90s is chosen to balance network overhead with timely failure detection.
			Time: synthetic_monitoring.HealthCheckInterval,

			// Timeout: Wait 30s for ping acknowledgment before considering connection dead.
			// If no ack is received within this window, the connection is closed and reconnected.
			// This timeout should be less than Time to avoid overlapping pings.
			Timeout: synthetic_monitoring.HealthCheckTimeout,

			// PermitWithoutStream: Allow pings even when no active RPCs are in flight.
			// This is critical for the agent because the GetChanges stream may be idle
			// between updates, but we still need to detect if the connection is broken.
			// Without this, pings would only occur during active RPCs, defeating the purpose.
			PermitWithoutStream: true,
		}),
	}

	transportCreds := insecure.NewCredentials()
	if !allowInsecure {
		transportCreds = credentials.NewTLS(&tls.Config{
			ServerName: grpcApiHost(addr),
			MinVersion: tls.VersionTLS12, // Enforce TLS 1.2 minimum for security
			// TLS 1.2 is widely supported and secure. TLS 1.0 and 1.1 are deprecated.
			// TLS 1.3 is preferred but TLS 1.2 ensures broader compatibility.
		})
	}
	opts = append(opts, grpc.WithTransportCredentials(transportCreds))

	return grpc.NewClient(addr, opts...)
}

func grpcApiHost(addr string) string {
	colonPos := strings.LastIndex(addr, ":")
	if colonPos == -1 {
		colonPos = len(addr)
	}
	hostname := addr[:colonPos]
	return hostname
}

type creds struct {
	Token         string
	AllowInsecure bool
}

func (c creds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + c.Token,
	}, nil
}

func (c creds) RequireTransportSecurity() bool {
	// Only require transport security when insecure mode is NOT enabled.
	// This allows the agent to use unencrypted connections for development/testing
	// when the -api-insecure flag is set, while enforcing TLS by default.
	return !c.AllowInsecure
}
