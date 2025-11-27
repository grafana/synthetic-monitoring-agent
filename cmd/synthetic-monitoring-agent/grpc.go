package main

import (
	"context"
	"crypto/tls"
	"strings"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

func dialAPIServer(addr string, allowInsecure bool, apiToken string) (*grpc.ClientConn, error) {
	apiCreds := creds{
		Token:         apiToken,
		AllowInsecure: allowInsecure,
	}

	opts := []grpc.DialOption{
		grpc.WithPerRPCCredentials(apiCreds),
		// Keep-alive is necessary to detect network failures in absence of writes from the client.
		// Without it, the agent would hang if the server disappears while waiting for a response.
		// See https://github.com/grpc/grpc/blob/master/doc/keepalive.md
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			// GRPC_ARG_KEEPALIVE_TIME_MS - Time between pings
			Time: synthetic_monitoring.HealthCheckInterval,
			// GRPC_ARG_KEEPALIVE_TIMEOUT_MS - Ping timeout
			Timeout: synthetic_monitoring.HealthCheckTimeout,
			// GRPC_ARG_KEEPALIVE_PERMIT_WITHOUT_CALLS - Allow pings even if no grpc calls are in place.
			// It shouldn't be the case that the agent doesn't have any calls in place during
			// a significant period of time.
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
