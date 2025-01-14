package main

import (
	"context"
	"crypto/tls"
	"log"
	"strings"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

func dialAPIServer(ctx context.Context, addr string, allowInsecure bool, apiToken string) (*grpc.ClientConn, error) {
	apiCreds := creds{Token: apiToken}

	opts := []grpc.DialOption{
		grpc.WithBlock(), //nolint:staticcheck,nolintlint // Will be removed in v2. TODO: Migrate to NewClient.
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
		transportCreds = credentials.NewTLS(&tls.Config{ServerName: grpcApiHost(addr)})
	}
	opts = append(opts, grpc.WithTransportCredentials(transportCreds))

	return grpc.DialContext(ctx, addr, opts...) //nolint:staticcheck,nolintlint // Will be removed in v2. TODO: Migrate to NewClient.
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
	Token string
}

func (c creds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + c.Token,
	}, nil
}

func (c creds) RequireTransportSecurity() bool {
	log.Printf("RequireTransportSecurity")
	// XXX(mem): this is true
	return false
}
