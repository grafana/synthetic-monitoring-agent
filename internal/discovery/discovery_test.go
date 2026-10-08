package discovery

import (
	"bytes"
	"net"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// stubLookups replaces the DNS lookup functions with fakes backed by hosts
// (name -> A/AAAA records) and srvs (name -> SRV records) for the duration of
// the test. Tests using it must not run in parallel.
func stubLookups(t *testing.T, hosts map[string][]string, srvs map[string][]*net.SRV) {
	t.Helper()

	origHost, origSRV := lookupHost, lookupSRV

	t.Cleanup(func() { lookupHost, lookupSRV = origHost, origSRV })

	lookupHost = func(host string) ([]string, error) {
		if ips, ok := hosts[host]; ok {
			return ips, nil
		}

		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	lookupSRV = func(_, _, name string) (string, []*net.SRV, error) {
		if records, ok := srvs[name]; ok {
			return name, records, nil
		}

		return "", nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
}

func TestNewDiscoverer_NoAddresses(t *testing.T) {
	discover, err := NewDiscoverer(nil, 7946, zerolog.Nop())
	require.NoError(t, err)

	addrs, err := discover()
	require.NoError(t, err)
	require.Empty(t, addrs)
}

func TestNewDiscoverer_RejectsInvalidPort(t *testing.T) {
	for _, entry := range []string{
		"10.0.0.1:",      // empty port
		"10.0.0.1:0",     // zero port
		"10.0.0.1:abc",   // non-numeric port
		"10.0.0.1:http",  // service name port
		"10.0.0.1:65536", // port out of range
		"[::1]:",         // ipv6 empty port
	} {
		_, err := NewDiscoverer([]string{"10.0.0.2:7946", entry}, 7946, zerolog.Nop())
		require.ErrorContainsf(t, err, "invalid port", "entry %q", entry)
	}
}

func TestStaticAddresses_Dedupe(t *testing.T) {
	discover, err := NewDiscoverer([]string{
		"10.0.0.1:7946",
		"10.0.0.2:7946",
		"10.0.0.1", // duplicate once the default port is added
	}, 7946, zerolog.Nop())
	require.NoError(t, err)

	addrs, err := discover()
	require.NoError(t, err)
	require.Equal(t, []string{"10.0.0.1:7946", "10.0.0.2:7946"}, addrs)
}

func TestStaticAddresses_PartialFailure(t *testing.T) {
	stubLookups(t,
		map[string][]string{"sm-agent": {"10.0.0.1"}},
		map[string][]*net.SRV{"empty-srv": {}},
	)

	var logs bytes.Buffer

	// One failing entry does not discard the addresses found by the others,
	// and is logged.
	discover, err := NewDiscoverer([]string{"sm-agent", "no-such-host"}, 7946, zerolog.New(&logs))
	require.NoError(t, err)

	addrs, err := discover()
	require.NoError(t, err)
	require.Equal(t, []string{"10.0.0.1:7946"}, addrs)
	require.Contains(t, logs.String(), "address did not resolve")
	require.Contains(t, logs.String(), "no-such-host")

	// An entry resolving to no addresses without an error, such as an SRV
	// name with no records, is logged too.
	logs.Reset()

	discover, err = NewDiscoverer([]string{"sm-agent", "dnssrv+empty-srv"}, 7946, zerolog.New(&logs))
	require.NoError(t, err)

	addrs, err = discover()
	require.NoError(t, err)
	require.Equal(t, []string{"10.0.0.1:7946"}, addrs)
	require.Contains(t, logs.String(), "address did not resolve")
	require.Contains(t, logs.String(), "dnssrv+empty-srv")

	// Nothing is logged when every entry resolves.
	logs.Reset()

	discover, err = NewDiscoverer([]string{"sm-agent"}, 7946, zerolog.New(&logs))
	require.NoError(t, err)

	_, err = discover()
	require.NoError(t, err)
	require.Empty(t, logs.String())

	// When nothing resolves, the errors are surfaced.
	discover, err = NewDiscoverer([]string{"no-such-host"}, 7946, zerolog.Nop())
	require.NoError(t, err)

	_, err = discover()
	require.Error(t, err)
}

func TestResolveAddress(t *testing.T) {
	stubLookups(t,
		map[string][]string{
			"sm-agent":      {"10.0.0.1", "10.0.0.2"},
			"sm-agent-0.sm": {"10.0.1.1"},
			"sm-agent-1.sm": {"10.0.1.2"},
		},
		map[string][]*net.SRV{
			"_gossip._tcp.sm": {
				{Target: "sm-agent-0.sm.", Port: 1234},
				{Target: "sm-agent-1.sm.", Port: 1234},
			},
			"srv-only": {{Target: "sm-agent-0.sm.", Port: 1234}},
		},
	)

	testcases := map[string]struct {
		entry    string
		expected []string
	}{
		"ipv4 with port":         {entry: "10.0.0.1:9000", expected: []string{"10.0.0.1:9000"}},
		"ipv4 without port":      {entry: "10.0.0.1", expected: []string{"10.0.0.1:7946"}},
		"ipv6 with port":         {entry: "[::1]:9000", expected: []string{"[::1]:9000"}},
		"ipv6 without port":      {entry: "::1", expected: []string{"[::1]:7946"}},
		"ipv6 brackets no port":  {entry: "[::1]", expected: []string{"[::1]:7946"}},
		"name with port":         {entry: "sm-agent:9000", expected: []string{"10.0.0.1:9000", "10.0.0.2:9000"}},
		"name without port":      {entry: "sm-agent", expected: []string{"10.0.0.1:7946", "10.0.0.2:7946"}},
		"dns prefix":             {entry: "dns+sm-agent", expected: []string{"10.0.0.1:7946", "10.0.0.2:7946"}},
		"dnssrv prefix":          {entry: "dnssrv+_gossip._tcp.sm", expected: []string{"10.0.1.1:7946", "10.0.1.2:7946"}},
		"dnssrvnoa prefix":       {entry: "dnssrvnoa+_gossip._tcp.sm:9000", expected: []string{"sm-agent-0.sm:9000", "sm-agent-1.sm:9000"}},
		"srv fallback no prefix": {entry: "srv-only", expected: []string{"10.0.1.1:7946"}},
		"surrounding whitespace": {entry: "  10.0.0.1  ", expected: []string{"10.0.0.1:7946"}},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := resolveAddress(tc.entry, "7946")
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestResolveAddress_Errors(t *testing.T) {
	stubLookups(t, nil, map[string][]*net.SRV{"srv-only": {{Target: "sm-agent-0.sm."}}})

	for _, entry := range []string{
		"no-such-host",    // neither A/AAAA nor SRV
		"dns+srv-only",    // explicit prefix: no SRV fallback
		"dnssrv+no-such",  // no SRV records
		"dnssrv+srv-only", // SRV target does not resolve
	} {
		_, err := resolveAddress(entry, "7946")
		require.Errorf(t, err, "entry %q", entry)
	}
}
