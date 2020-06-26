package main

import (
	"context"
	"crypto/tls"
	"log"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func dialAPIServer(ctx context.Context, addr string, insecure bool, apiToken string) (*grpc.ClientConn, error) {
	apiCreds := creds{Token: apiToken}

	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithPerRPCCredentials(apiCreds),
	}

	if insecure {
		opts = append(opts, grpc.WithInsecure())
	} else {
		creds := credentials.NewTLS(&tls.Config{ServerName: grpcApiHost(addr)})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}

	return grpc.DialContext(ctx, addr, opts...)
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
