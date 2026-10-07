// Package discovery resolves the addresses of a fleet's members, such as
// cluster peers or browser pool instances.
package discovery

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
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
)

// NewDiscoverer builds a DiscoverFn from static addresses. With none set the
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
// Addresses without a port are joined with defaultPort. SRV record ports are
// ignored: every member is expected to listen on the same port.
//
// An entry that resolves to no addresses, whether from a lookup error or an
// empty result, is logged as a warning and skipped; the DiscoverFn returns an
// error only when nothing resolves.
//
// TODO: consider supporting go-discover providers (k8s, AWS, GCE, Azure, ...)
// as an alternative to static addresses.
func NewDiscoverer(addresses []string, defaultPort int, logger zerolog.Logger) (DiscoverFn, error) {
	port := strconv.Itoa(defaultPort)

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
			if len(resolved) == 0 {
				// A failing entry should not discard the addresses found by the
				// others. Log a warning instead.
				logger.Warn().Err(err).Str("address", e).Msg("address did not resolve")
			}

			if err != nil {
				errs = append(errs, err)
				continue
			}
			addrs = append(addrs, resolved...)
		}
		addrs = dedupe(addrs)
		// Surface errors only when nothing resolved: a single failing entry
		// (e.g. a transient DNS blip) is logged above rather than discarding the
		// addresses found by the others, and periodic re-invocation recovers on
		// the next tick.
		if len(addrs) == 0 && len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
		return addrs, nil
	}, nil
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
