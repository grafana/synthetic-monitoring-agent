package cluster

// Peer discovery (DiscoverFn, NewDiscoverer, DNS/k8s resolution) lives in
// internal/discovery. This file retains only the ckit-specific advertise-
// address helper.

import (
	"fmt"
	"net"
	"strconv"

	"github.com/grafana/ckit/advertise"
)

// AdvertiseAddress resolves the host:port this node advertises to peers. If
// interfaces is empty, advertise.DefaultInterfaces (eth0, en0) is used; the
// first usable address on the first matching interface is chosen.
func AdvertiseAddress(interfaces []string, port int) (string, error) {
	if len(interfaces) == 0 {
		interfaces = advertise.DefaultInterfaces
	}
	addr, err := advertise.FirstAddress(interfaces)
	if err != nil {
		return "", fmt.Errorf("cluster: resolving advertise address: %w", err)
	}
	return net.JoinHostPort(addr.String(), strconv.Itoa(port)), nil
}
