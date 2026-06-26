package cluster

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"

	"golang.org/x/net/http2"
)

// NewGossipClient builds the *http.Client ckit uses to dial peers over
// unencrypted (h2c) HTTP/2: the transport speaks HTTP/2 with prior knowledge
// and dials plaintext TCP instead of TLS. Pass the result as RingConfig.Client.
//
// Gossip runs unencrypted because it is trusted intra-cluster traffic
// (pod-to-pod) and avoiding TLS removes all cert lifecycle management from the
// initial implementation.
//
// AllowHTTP lets the transport use the plaintext "http" scheme, but per the
// http2.Transport docs that alone does not enable h2c. The working idiom is to
// also override DialTLSContext: with AllowHTTP set, the transport establishes
// every connection through DialTLSContext, so returning a plain TCP conn there
// (instead of a TLS handshake) yields HTTP/2 with prior knowledge over
// cleartext. The field is named for TLS only because it is the
// connection-setup seam; here it carries the plaintext path.
//
// TODO: TLS support (-cluster.enable-tls). To run gossip over TLS, set
// AllowHTTP to false, populate TLSClientConfig from the configured CA/cert/key,
// and replace DialTLSContext with a real tls.DialWithDialer. The server side
// (NewGossipServer) must then stop advertising unencrypted HTTP/2.
func NewGossipClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
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
