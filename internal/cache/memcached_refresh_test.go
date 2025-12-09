package cache

import (
	"context"
	"net"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRefreshServers_PeriodicRefresh tests that DNS resolution happens periodically.
func TestRefreshServers_PeriodicRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resolver := newMockResolver()
		resolver.setResponse("memcached.example.com", "10.0.1.1", "10.0.1.2")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client, err := NewMemcachedClient(ctx, MemcachedConfig{
			Servers:         []string{"memcached.example.com:11211"},
			Logger:          testLogger(),
			RefreshInterval: 10 * time.Second,
			Resolver:        resolver,
		})
		require.NoError(t, err)
		require.NotNil(t, client)

		// Wait for the client to be initialized
		synctest.Wait()

		// Clear initial lookup
		resolver.clearLookups()

		// Change DNS response
		resolver.setResponse("memcached.example.com", "10.0.2.1", "10.0.2.2", "10.0.2.3")

		// Advance time by 10 seconds to trigger first refresh
		time.Sleep(10 * time.Second)
		synctest.Wait()

		// Verify DNS was queried
		lookups := resolver.getLookups()
		require.Contains(t, lookups, "memcached.example.com")

		// Verify server list was updated with new IPs
		var addrs []string
		err = client.serverList.Each(func(addr net.Addr) error {
			addrs = append(addrs, addr.String())
			return nil
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"10.0.2.1:11211", "10.0.2.2:11211", "10.0.2.3:11211"}, addrs)

		// Clear lookups and advance time again
		resolver.clearLookups()
		resolver.setResponse("memcached.example.com", "10.0.3.1")

		// Allow another refresh to happen.
		time.Sleep(10 * time.Second)
		synctest.Wait()

		// Verify second refresh happened
		lookups = resolver.getLookups()
		require.Contains(t, lookups, "memcached.example.com")

		// Verify server list updated again
		addrs = nil
		err = client.serverList.Each(func(addr net.Addr) error {
			addrs = append(addrs, addr.String())
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, []string{"10.0.3.1:11211"}, addrs)

		// Cancel context to stop refresh
		cancel()
		synctest.Wait()
	})
}

// TestRefreshServers_MultipleHosts tests refreshing multiple hostnames.
func TestRefreshServers_MultipleHosts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resolver := newMockResolver()
		resolver.setResponse("cache1.example.com", "10.0.1.1")
		resolver.setResponse("cache2.example.com", "10.0.2.1", "10.0.2.2")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client, err := NewMemcachedClient(ctx, MemcachedConfig{
			Servers: []string{
				"cache1.example.com:11211",
				"cache2.example.com:11211",
				"192.168.1.1:11211", // IP address should be used directly
			},
			Logger:          testLogger(),
			RefreshInterval: 5 * time.Second,
			Resolver:        resolver,
		})
		require.NoError(t, err)
		require.NotNil(t, client)

		synctest.Wait()

		// Verify initial server list
		var addrs []string
		err = client.serverList.Each(func(addr net.Addr) error {
			addrs = append(addrs, addr.String())
			return nil
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{
			"10.0.1.1:11211",
			"10.0.2.1:11211",
			"10.0.2.2:11211",
			"192.168.1.1:11211",
		}, addrs)

		// Update DNS responses
		resolver.clearLookups()
		resolver.setResponse("cache1.example.com", "10.0.1.1", "10.0.1.2")
		resolver.setResponse("cache2.example.com", "10.0.2.3")

		// Trigger refresh
		time.Sleep(5 * time.Second)
		synctest.Wait()

		// Verify lookups happened for hostnames but not IP
		lookups := resolver.getLookups()
		require.Contains(t, lookups, "cache1.example.com")
		require.Contains(t, lookups, "cache2.example.com")
		require.NotContains(t, lookups, "192.168.1.1")

		// Verify updated server list
		addrs = nil
		err = client.serverList.Each(func(addr net.Addr) error {
			addrs = append(addrs, addr.String())
			return nil
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{
			"10.0.1.1:11211",
			"10.0.1.2:11211",
			"10.0.2.3:11211",
			"192.168.1.1:11211",
		}, addrs)

		cancel()
		synctest.Wait()
	})
}

// TestRefreshServers_DNSFailure tests handling of DNS resolution failures.
func TestRefreshServers_DNSFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resolver := newMockResolver()
		resolver.setResponse("cache.example.com", "10.0.1.1", "10.0.1.2")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client, err := NewMemcachedClient(ctx, MemcachedConfig{
			Servers:         []string{"cache.example.com:11211"},
			Logger:          testLogger(),
			RefreshInterval: 5 * time.Second,
			Resolver:        resolver,
		})
		require.NoError(t, err)
		synctest.Wait()

		// Verify initial servers
		var addrs []string
		err = client.serverList.Each(func(addr net.Addr) error {
			addrs = append(addrs, addr.String())
			return nil
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"10.0.1.1:11211", "10.0.1.2:11211"}, addrs)

		// Make DNS fail
		resolver.setError("cache.example.com", &net.DNSError{
			Err:         "temporary failure",
			Name:        "cache.example.com",
			IsTemporary: true,
		})

		// Trigger refresh
		time.Sleep(5 * time.Second)
		synctest.Wait()

		// Verify servers remain unchanged (old servers still work)
		addrs = nil
		err = client.serverList.Each(func(addr net.Addr) error {
			addrs = append(addrs, addr.String())
			return nil
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"10.0.1.1:11211", "10.0.1.2:11211"}, addrs)

		// Restore DNS and add new IP
		resolver.setResponse("cache.example.com", "10.0.2.1")

		// Trigger another refresh
		time.Sleep(5 * time.Second)
		synctest.Wait()

		// Verify servers updated to new IP
		addrs = nil
		err = client.serverList.Each(func(addr net.Addr) error {
			addrs = append(addrs, addr.String())
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, []string{"10.0.2.1:11211"}, addrs)

		cancel()
		synctest.Wait()
	})
}

// TestRefreshServers_ContextCancellation tests that refresh stops when context is cancelled.
func TestRefreshServers_ContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resolver := newMockResolver()
		resolver.setResponse("cache.example.com", "10.0.1.1")

		ctx, cancel := context.WithCancel(context.Background())

		_, err := NewMemcachedClient(ctx, MemcachedConfig{
			Servers:         []string{"cache.example.com:11211"},
			Logger:          testLogger(),
			RefreshInterval: 5 * time.Second,
			Resolver:        resolver,
		})
		require.NoError(t, err)
		synctest.Wait()

		// Clear lookups
		resolver.clearLookups()

		// Advance time less than refresh interval, then cancel
		time.Sleep(3 * time.Second)
		cancel()

		// Give the refresh goroutine time to process cancellation
		synctest.Wait()

		// Verify no refresh happened (context cancelled before 5 second interval)
		lookups := resolver.getLookups()
		require.Empty(t, lookups, "No DNS lookups should happen after context cancellation")
	})
}

// TestRefreshServers_CustomInterval tests that custom refresh intervals work.
func TestRefreshServers_CustomInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resolver := newMockResolver()
		resolver.setResponse("cache.example.com", "10.0.1.1")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, err := NewMemcachedClient(ctx, MemcachedConfig{
			Servers:         []string{"cache.example.com:11211"},
			Logger:          testLogger(),
			RefreshInterval: 30 * time.Second, // Custom interval
			Resolver:        resolver,
		})
		require.NoError(t, err)
		synctest.Wait()

		resolver.clearLookups()

		// Advance time less than interval - should not refresh
		time.Sleep(20 * time.Second)
		synctest.Wait()

		lookups := resolver.getLookups()
		require.Empty(t, lookups, "Should not refresh before interval")

		// Advance past interval
		time.Sleep(15 * time.Second) // Total: 35 seconds
		synctest.Wait()

		lookups = resolver.getLookups()
		require.Contains(t, lookups, "cache.example.com", "Should refresh after interval")

		cancel()
		synctest.Wait()
	})
}

// TestResolveServers_DirectIPAddresses tests that IP addresses are used without resolution.
func TestResolveServers_DirectIPAddresses(t *testing.T) {
	resolver := newMockResolver()
	logger := testLogger()

	servers := []string{
		"192.168.1.1:11211",
		"10.0.0.1:11211",
		"[2001:db8::1]:11211",
	}

	resolved := resolveServers(context.Background(), servers, resolver, logger)

	// Verify IP addresses are returned as-is
	require.ElementsMatch(t, servers, resolved)

	// Verify no DNS lookups happened
	lookups := resolver.getLookups()
	require.Empty(t, lookups, "IP addresses should not trigger DNS lookups")
}

// TestResolveServers_MixedHostnamesAndIPs tests resolving a mix of hostnames and IPs.
func TestResolveServers_MixedHostnamesAndIPs(t *testing.T) {
	resolver := newMockResolver()
	resolver.setResponse("cache.example.com", "10.1.1.1", "10.1.1.2")
	logger := testLogger()

	servers := []string{
		"cache.example.com:11211",
		"192.168.1.1:11211",
		"[::1]:11211",
	}

	resolved := resolveServers(context.Background(), servers, resolver, logger)

	require.ElementsMatch(t, []string{
		"10.1.1.1:11211",
		"10.1.1.2:11211",
		"192.168.1.1:11211",
		"[::1]:11211",
	}, resolved)

	// Verify only hostname was looked up
	lookups := resolver.getLookups()
	require.Equal(t, []string{"cache.example.com"}, lookups)
}

// mockResolver is a controllable DNS resolver for testing.
type mockResolver struct {
	mu        sync.RWMutex
	responses map[string][]net.IP
	errors    map[string]error
	lookups   []string // Track lookup history
}

func newMockResolver() *mockResolver {
	return &mockResolver{
		responses: make(map[string][]net.IP),
		errors:    make(map[string]error),
		lookups:   make([]string, 0),
	}
}

func (m *mockResolver) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lookups = append(m.lookups, host)

	if err, ok := m.errors[host]; ok {
		return nil, err
	}

	if ips, ok := m.responses[host]; ok {
		return ips, nil
	}

	return nil, &net.DNSError{
		Err:        "no such host",
		Name:       host,
		IsNotFound: true,
	}
}

// setResponse sets the IP addresses that will be returned for a given hostname.
// This also clears any error that was previously set for this host.
func (m *mockResolver) setResponse(host string, ips ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	netIPs := make([]net.IP, len(ips))
	for i, ip := range ips {
		netIPs[i] = net.ParseIP(ip)
	}

	m.responses[host] = netIPs
	delete(m.errors, host) // Clear any error for this host
}

// setError sets an error that will be returned for a given hostname.
func (m *mockResolver) setError(host string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.errors[host] = err
}

// clearLookups clears the lookup history.
func (m *mockResolver) clearLookups() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lookups = nil
}

// getLookups returns the list of hostnames that were looked up.
func (m *mockResolver) getLookups() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return slices.Clone(m.lookups)
}
