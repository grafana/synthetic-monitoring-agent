package cluster

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGossipTransport verifies the client built by NewGossipClient negotiates
// unencrypted HTTP/2 end-to-end against the server built by NewGossipServer,
// the property ckit's gossip transport requires.
func TestGossipTransport(t *testing.T) {
	const route = "/api/v1/ckit/transport/"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, 2, r.ProtoMajor, "gossip handler must be reached over HTTP/2")
		w.WriteHeader(http.StatusOK)
	})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := NewGossipServer(route, handler)
	defer func() { _ = srv.Shutdown(context.Background()) }()
	go func() { _ = srv.Run(lis) }()

	client := NewGossipClient()
	resp, err := client.Get("http://" + lis.Addr().String() + route)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, resp.ProtoMajor, "response must be served over HTTP/2")
}
