package cluster

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// NewGossipClient builds the *http.Client ckit uses to dial peers over
// unencrypted (h2c) HTTP/2: the transport speaks HTTP/2 with prior knowledge
// and dials plaintext TCP instead of TLS. Pass the result as RingConfig.Client.
//
// Gossip runs unencrypted because it is trusted intra-cluster traffic
// (pod-to-pod) and avoiding TLS removes all cert lifecycle management from the
// initial implementation.
//
// Only unencrypted HTTP/2 is enabled: with HTTP/1 disabled, the transport
// speaks HTTP/2 with prior knowledge on the plaintext conn from DialContext for
// "http" URLs, instead of falling back to HTTP/1.1.
//
// TODO: TLS support (-cluster.enable-tls). To run gossip over TLS, enable
// HTTP/2 (SetHTTP2) instead of unencrypted HTTP/2 and populate TLSClientConfig
// from the configured CA/cert/key; the transport does the TLS handshake itself.
// The server side (NewGossipServer) must then stop advertising unencrypted
// HTTP/2.
func NewGossipClient() *http.Client {
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)

	return &http.Client{
		Transport: &http.Transport{
			Protocols: protocols,
			// Cap connection establishment so a dial to a gone or black-holed peer fails
			// fast instead of hanging on the OS TCP timeout when the caller's context has
			// no deadline. This bounds only the dial, not the connection lifetime, so
			// long-lived /stream connections are unaffected. DialContext still honors ctx
			// cancellation and any earlier deadline.
			//
			// TODO: Consider making this timeout configurable.
			DialContext: (&net.Dialer{Timeout: 30 * time.Second}).DialContext,
		},
	}
}

// GossipServer serves HTTP/2 traffic on a listener.
type GossipServer struct {
	srv *http.Server
}

// NewGossipServer returns a GossipServer preconfigured to serve ckit gossip
// traffic over unencrypted HTTP/2. It has no WriteTimeout so ckit's long-lived
// bidirectional /stream connections are not killed mid-flight; do not add one.
// route and handler come from RingNode.Handler().
//
// SetUnencryptedHTTP2 makes the server accept h2c (HTTP/2 with prior knowledge)
// on a plaintext listener, matching the h2c client in NewGossipClient.
//
// TODO: TLS support (-cluster.enable-tls). To serve gossip over TLS, drop the
// unencrypted-HTTP/2 protocol and give the server a TLSConfig built from the
// configured CA/cert/key, paired with the TLS client dialer in NewGossipClient.
func NewGossipServer(route string, handler http.Handler) *GossipServer {
	mux := http.NewServeMux()
	mux.Handle(route, handler)

	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)

	return &GossipServer{
		srv: &http.Server{
			Handler:   mux,
			Protocols: protocols,
		},
	}
}

// Run serves traffic on l until Shutdown is called.
func (s *GossipServer) Run(l net.Listener) error {
	if err := s.srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("could not serve cluster gossip on %s: %w", l.Addr(), err)
	}

	return nil
}

// Shutdown gracefully shuts down the server without interrupting active
// connections, bounded by ctx.
func (s *GossipServer) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
