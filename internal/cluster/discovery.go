package cluster

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/grafana/ckit/advertise"
	"github.com/hashicorp/go-discover/provider/k8s"
)

// DiscoverFn resolves the current set of peer addresses the node should try to
// join. It is called once at startup and then re-invoked on RejoinInterval (see
// RingNode.Start) so the ring picks up scale-ups and restarted peers; ckit's
// Start is additive, so returning the full current set on every call is correct.
type DiscoverFn func() ([]string, error)

// NewDiscoverer builds a DiscoverFn from a list of join entries. Each entry is
// resolved independently and the results are unioned and de-duplicated. Two
// entry forms are supported:
//
//   - go-discover config ("provider=k8s namespace=... label_selector=..."):
//     pod discovery via the Kubernetes API.
//   - host[:port] / ip[:port]: DNS resolution. A literal IP is passed through; a
//     hostname is expanded to every A/AAAA record and each is joined with the
//     supplied port. This is how a headless Service is resolved: the Service
//     name resolves to the set of ready pod IPs.
//
// logger is used by the k8s provider for debug output; if nil, output is
// discarded.
//
// TODO: support the other go-discover providers (AWS/GCE/Azure/DigitalOcean/
// ...) and DNS-SD / SRV entries. They are intentionally omitted for now so the
// agent does not pull the full cloud-provider SDK dependency tree (only the k8s
// provider, and thus only client-go, is imported). Add them explicitly as
// deployment targets require.
func NewDiscoverer(entries []string, logger *log.Logger) (DiscoverFn, error) {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	// Validate provider entries up front so misconfiguration fails fast at
	// startup rather than on the first discovery tick.
	for _, e := range entries {
		if isProviderConfig(e) {
			if p := parseConfig(e)["provider"]; p != "k8s" {
				return nil, fmt.Errorf("cluster: unsupported discovery provider %q (only %q is supported)", p, "k8s")
			}
		}
	}

	return func() ([]string, error) {
		var (
			addrs []string
			errs  []error
		)
		for _, e := range entries {
			resolved, err := resolveEntry(e, logger)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			addrs = append(addrs, resolved...)
		}
		addrs = dedupe(addrs)
		// Surface errors only when nothing resolved: a single failing entry
		// (e.g. a transient DNS blip) should not discard peers found by the
		// others, and ckit's additive Start recovers on the next tick.
		if len(addrs) == 0 && len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
		return addrs, nil
	}, nil
}

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

func resolveEntry(entry string, logger *log.Logger) ([]string, error) {
	entry = strings.TrimSpace(entry)
	if isProviderConfig(entry) {
		// Only k8s is reachable here: NewDiscoverer rejects other providers.
		return (&k8s.Provider{}).Addrs(parseConfig(entry), logger)
	}
	return resolveDNS(entry)
}

func isProviderConfig(entry string) bool {
	return strings.Contains(entry, "provider=")
}

// parseConfig parses a go-discover "key=val key=val" string into an args map.
// Pairs are whitespace-separated and values are split on the first '=' (so
// values may themselves contain '=', e.g. label_selector=app=sm-agent).
//
// TODO: this does not handle quoted values containing spaces (e.g. a selector
// with a space). go-discover's quoting-aware parser lives in its top-level
// package, which transitively imports every provider SDK; reproducing only the
// quote handling here is the lighter-weight path if it is ever needed.
func parseConfig(s string) map[string]string {
	args := make(map[string]string)
	for field := range strings.FieldsSeq(s) {
		if k, v, ok := strings.Cut(field, "="); ok {
			args[k] = v
		}
	}
	return args
}

// resolveDNS resolves a host[:port] entry to peer addresses. A literal IP is
// returned unchanged; a hostname is expanded to every A/AAAA record, each
// joined with the supplied port (if any). Expanding all records is what makes a
// headless Service name resolve to all of its backing pod addresses.
func resolveDNS(entry string) ([]string, error) {
	host, port, err := net.SplitHostPort(entry)
	if err != nil {
		// No port component; treat the whole entry as the host.
		host, port = entry, ""
	}

	if net.ParseIP(host) != nil {
		return []string{entry}, nil
	}

	ips, err := net.LookupHost(host)
	if err != nil {
		return nil, fmt.Errorf("cluster: resolving %q: %w", entry, err)
	}

	addrs := make([]string, 0, len(ips))
	for _, ip := range ips {
		if port != "" {
			addrs = append(addrs, net.JoinHostPort(ip, port))
		} else {
			addrs = append(addrs, ip)
		}
	}
	return addrs, nil
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
