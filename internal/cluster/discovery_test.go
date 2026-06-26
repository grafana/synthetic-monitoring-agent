package cluster

import (
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	// Values may contain '=' (e.g. label selectors); only the first '=' splits.
	got := parseConfig("provider=k8s namespace=sm label_selector=app=sm-agent")
	require.Equal(t, map[string]string{
		"provider":       "k8s",
		"namespace":      "sm",
		"label_selector": "app=sm-agent",
	}, got)
}

func TestIsProviderConfig(t *testing.T) {
	require.True(t, isProviderConfig("provider=k8s namespace=sm"))
	require.False(t, isProviderConfig("sm-agent.sm.svc.cluster.local:7946"))
	require.False(t, isProviderConfig("10.0.0.1:7946"))
}

func TestNewDiscoverer_RejectsUnsupportedProvider(t *testing.T) {
	_, err := NewDiscoverer([]string{"provider=aws tag_key=cluster"}, nil)
	require.Error(t, err)
}

func TestNewDiscoverer_AcceptsK8sProvider(t *testing.T) {
	// Validation only: resolution would need a real cluster, so the DiscoverFn
	// is not invoked here.
	_, err := NewDiscoverer([]string{"provider=k8s namespace=sm"}, nil)
	require.NoError(t, err)
}

func TestDiscoverFn_StaticAddressesAndDedupe(t *testing.T) {
	discover, err := NewDiscoverer([]string{
		"10.0.0.1:7946",
		"10.0.0.2:7946",
		"10.0.0.1:7946", // duplicate
	}, nil)
	require.NoError(t, err)

	addrs, err := discover()
	require.NoError(t, err)
	require.Equal(t, []string{"10.0.0.1:7946", "10.0.0.2:7946"}, addrs)
}

func TestResolveDNS_LiteralIPPassthrough(t *testing.T) {
	for _, entry := range []string{"10.0.0.1:7946", "10.0.0.1", "::1", "[::1]:7946"} {
		got, err := resolveDNS(entry)
		require.NoError(t, err)
		require.Equal(t, []string{entry}, got)
	}
}

func TestResolveDNS_HostnameExpansion(t *testing.T) {
	got, err := resolveDNS("localhost:7946")
	require.NoError(t, err)
	require.NotEmpty(t, got)
	require.Contains(t, got, "127.0.0.1:7946")
}

func TestResolveDNS_UnresolvableHost(t *testing.T) {
	_, err := resolveDNS("no-such-host.invalid:7946")
	require.Error(t, err)
}

func TestAdvertiseAddress_JoinsPort(t *testing.T) {
	addr, err := AdvertiseAddress(nil, 7946)
	if err != nil {
		// No usable interface in this environment (e.g. CI without eth0/en0).
		t.Skipf("advertise address unavailable: %v", err)
	}
	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	require.Equal(t, strconv.Itoa(7946), port)
}
