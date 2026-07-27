// Package discovery resolves the addresses of a fleet's members, such as
// cluster peers or browser pool instances.
package discovery

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/hashicorp/go-discover/provider/k8s"
)

// DiscoverFn returns the current addresses (host:port) of a fleet's members.
// Callers may invoke it repeatedly to pick up membership changes, such as
// scale-ups and restarted members; a failed invocation can be retried on the
// next one.
type DiscoverFn func() ([]string, error)

// Lookup functions used for resolution; replaced in tests.
var (
	lookupHost = net.LookupHost
	lookupSRV  = net.LookupSRV
	k8sAddrs   = (&k8s.Provider{}).Addrs
)

// NewDiscoverer builds a DiscoverFn from either static addresses or a
// go-discover config. The two are mutually exclusive; with neither set the
// DiscoverFn returns no addresses, and what that means is up to the caller
// (e.g. a cluster node bootstraps a new cluster).
//
// addresses are [prefix+]host[:port] entries. Each is resolved
// independently and the results are unioned and de-duplicated. A literal IP is
// passed through; a name is resolved according to its prefix:
//
//   - dns+name: A/AAAA records.
//   - dnssrv+name: SRV records, each target then resolved as A/AAAA.
//   - dnssrvnoa+name: SRV records, targets used as-is.
//   - no prefix: A/AAAA records, falling back to SRV (then A/AAAA) when the
//     name has none. This is how a headless Service is resolved: the Service
//     name resolves to the set of ready pod IPs.
//
// discoverConfig is a go-discover config ("provider=k8s namespace=...
// label_selector=..."): pod discovery via the Kubernetes API.
//
// Addresses without a port are joined with defaultPort. SRV record ports are
// ignored: every member is expected to listen on the same port.
//
// TODO: support the other go-discover providers (AWS/GCE/Azure/DigitalOcean/
// ...). They are intentionally omitted for now so the agent does not pull the
// full cloud-provider SDK dependency tree (only the k8s provider, and thus only
// client-go, is imported). Add them explicitly as deployment targets require.
func NewDiscoverer(addresses []string, discoverConfig string, defaultPort int) (DiscoverFn, error) {
	if len(addresses) > 0 && discoverConfig != "" {
		return nil, errors.New("discovery: at most one of static addresses and a discover config may be set")
	}

	port := strconv.Itoa(defaultPort)

	if discoverConfig != "" {
		args := parseConfig(discoverConfig)
		// Validate the provider up front so misconfiguration fails fast at
		// startup rather than on the first discovery tick.
		if p := args["provider"]; p != "k8s" {
			return nil, fmt.Errorf("discovery: unsupported discovery provider %q (only k8s is supported)", p)
		}

		// TODO: the k8s provider's debug output (why pods are skipped: not running,
		// not ready, missing port) is discarded. Route it to a debug-level logger if
		// it is ever needed to troubleshoot discovery.
		logger := log.New(io.Discard, "", 0)

		return func() ([]string, error) {
			addrs, err := k8sAddrs(args, logger)
			if err != nil {
				return nil, err
			}
			for i, addr := range addrs {
				addrs[i] = appendPortIfAbsent(addr, port)
			}
			return dedupe(addrs), nil
		}, nil
	}

	// Validate ports up front so a typo fails fast at startup, rather than
	// producing an unreachable address that callers may skip without notice.
	for _, e := range addresses {
		if _, p, err := net.SplitHostPort(strings.TrimSpace(e)); err == nil {
			if n, err := strconv.ParseUint(p, 10, 16); err != nil || n == 0 {
				return nil, fmt.Errorf("discovery: invalid port in address %q", e)
			}
		}
	}

	return func() ([]string, error) {
		var (
			addrs []string
			errs  []error
		)
		for _, e := range addresses {
			resolved, err := resolveAddress(e, port)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			addrs = append(addrs, resolved...)
		}
		addrs = dedupe(addrs)
		// Surface errors only when nothing resolved: a single failing entry
		// (e.g. a transient DNS blip) should not discard addresses found by the
		// others, and periodic re-invocation recovers on the next tick.
		if len(addrs) == 0 && len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
		return addrs, nil
	}, nil
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

// resolveAddress resolves a [prefix+]host[:port] address to the member
// addresses it expands to, each joined with the entry's port, or defaultPort if
// it has none.
func resolveAddress(entry, defaultPort string) ([]string, error) {
	entry = strings.TrimSpace(entry)

	host, port, err := net.SplitHostPort(entry)
	if err != nil {
		// No port component; treat the whole entry as the host. Trim brackets
		// from an IPv6 address like "[::1]": JoinHostPort adds its own.
		host, port = strings.Trim(entry, "[]"), defaultPort
	}

	hosts, err := resolveHost(host)
	if err != nil {
		return nil, fmt.Errorf("discovery: resolving %q: %w", entry, err)
	}

	addrs := make([]string, 0, len(hosts))
	for _, h := range hosts {
		addrs = append(addrs, net.JoinHostPort(h, port))
	}
	return addrs, nil
}

// resolveHost resolves an address host according to its prefix (see
// NewDiscoverer). Expanding all records is what makes a headless Service name
// resolve to all of its backing pod addresses.
func resolveHost(host string) ([]string, error) {
	if name, ok := strings.CutPrefix(host, "dns+"); ok {
		return lookupHost(name)
	}
	if name, ok := strings.CutPrefix(host, "dnssrv+"); ok {
		return resolveSRV(name, true)
	}
	if name, ok := strings.CutPrefix(host, "dnssrvnoa+"); ok {
		return resolveSRV(name, false)
	}

	if net.ParseIP(host) != nil {
		return []string{host}, nil
	}

	ips, err := lookupHost(host)
	if err == nil {
		return ips, nil
	}
	hosts, srvErr := resolveSRV(host, true)
	if srvErr != nil {
		return nil, errors.Join(err, srvErr)
	}
	return hosts, nil
}

// resolveSRV looks up the SRV records for name and returns their targets,
// resolved to A/AAAA records when resolveTargets is set. Targets that fail to
// resolve are skipped; an error is returned only when none resolve.
func resolveSRV(name string, resolveTargets bool) ([]string, error) {
	_, records, err := lookupSRV("", "", name)
	if err != nil {
		return nil, err
	}

	var (
		hosts []string
		errs  []error
	)
	for _, record := range records {
		target := strings.TrimSuffix(record.Target, ".")
		if !resolveTargets {
			hosts = append(hosts, target)
			continue
		}

		ips, err := lookupHost(target)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		hosts = append(hosts, ips...)
	}
	if len(hosts) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return hosts, nil
}

// appendPortIfAbsent joins addr with port unless addr already has one.
func appendPortIfAbsent(addr, port string) string {
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	return net.JoinHostPort(strings.Trim(addr, "[]"), port)
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
