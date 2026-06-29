package cluster

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/grafana/ckit/peer"
	"github.com/grafana/ckit/shard"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
)

func TestMonoNode(t *testing.T) {
	n := NewMono()

	require.True(t, n.Ready())

	for _, id := range []model.GlobalID{0, 1, 42, 1000001} {
		owner, err := n.IsOwner(id)
		require.NoError(t, err)
		require.True(t, owner, "mono node must own every check (id %d)", id)
	}
}

// TestNewRingNode verifies a Ring can be constructed and exposes its ckit handler
// and metrics. It does not start gossip; multi-node behavior is covered by the
// integration test (item 14).
func TestNewRingNode(t *testing.T) {
	r, err := NewRingNode(RingConfig{
		Name:          "test-node",
		AdvertiseAddr: "127.0.0.1:7946",
		Client:        &http.Client{},
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, r)

	var _ Node = r

	route, h := r.Handler()
	require.NotEmpty(t, route)
	require.NotNil(t, h)

	require.NotNil(t, r.Metrics())
}

// TestRingNodeSingleConsistentOwner asserts RF=1 (exactly one owner) and that
// every agent independently agrees on who that owner is.
func TestRingNodeSingleConsistentOwner(t *testing.T) {
	cluster := ringCluster("a", "b", "c")

	for id := model.GlobalID(1); id <= 500; id++ {
		owners := 0
		for _, n := range cluster {
			mine, err := n.IsOwner(id)
			require.NoError(t, err)
			if mine {
				owners++
			}
		}
		require.Equalf(t, 1, owners, "check %d must have exactly one owner", id)
	}
}

func TestRingNodeDeterministic(t *testing.T) {
	n := ringCluster("a", "b", "c")["a"]

	for id := model.GlobalID(1); id <= 200; id++ {
		want, err := n.IsOwner(id)
		require.NoError(t, err)
		for range 5 {
			got, err := n.IsOwner(id)
			require.NoError(t, err)
			require.Equalf(t, want, got, "IsOwner not deterministic for check %d", id)
		}
	}
}

func TestRingNodeDistribution(t *testing.T) {
	names := []string{"a", "b", "c"}
	cluster := ringCluster(names...)

	const total = 3000
	counts := make(map[string]int, len(names))
	for id := model.GlobalID(1); id <= total; id++ {
		owned := 0
		for self, n := range cluster {
			mine, err := n.IsOwner(id)
			require.NoError(t, err)
			if mine {
				counts[self]++
				owned++
			}
		}
		require.Equalf(t, 1, owned, "check %d must be owned exactly once", id)
	}

	for _, name := range names {
		// 512 tokens/node over 3 peers should land each within 35% of an even
		// third; a wide bound keeps the test stable while still catching gross
		// skew (e.g. one node owning everything).
		require.InEpsilonf(t, total/len(names), counts[name], 0.35,
			"peer %s owns %d of %d, expected ~%d", name, counts[name], total, total/len(names))
	}
}

func TestRingNodeNoEligiblePeers(t *testing.T) {
	n := &RingNode{sharder: shard.Ring(512)} // no peers set

	_, err := n.IsOwner(1)
	require.Error(t, err)
}

// TestRingNodeReadyFailOpen verifies that a trivially-small minimum cluster
// size makes the node ready immediately (a lone agent runs everything).
func TestRingNodeReadyFailOpen(t *testing.T) {
	for _, min := range []int{0, 1} {
		n := &RingNode{sharder: shard.Ring(512), minClusterSize: min}
		require.Truef(t, n.Ready(), "minClusterSize %d must be ready immediately", min)
	}
}

// TestRingNodeReadyLatches verifies the node becomes ready once it reaches the
// minimum cluster size and stays ready even if peers later drop below it.
func TestRingNodeReadyLatches(t *testing.T) {
	s := shard.Ring(512)
	n := &RingNode{sharder: s, minClusterSize: 3}

	require.False(t, n.Ready(), "must not be ready before reaching the minimum")

	s.SetPeers(participants("a", "a", "b", "c"))
	n.updateReadyState()
	require.True(t, n.Ready(), "must be ready once the minimum is reached")

	// A dip below the minimum must not un-ready the node (latching).
	s.SetPeers(participants("a", "a"))
	n.updateReadyState()
	require.True(t, n.Ready(), "readiness must latch despite dropping below the minimum")
}

// TestRingNodeReadyDeadline verifies the fail-open deadline: when the minimum is
// never reached, the wait-timeout makes the node ready and fires onChange once.
// It runs under synctest so the deadline fires on the fake clock, deterministically.
func TestRingNodeReadyDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		changed := make(chan struct{}, 4)
		n := &RingNode{
			sharder:        shard.Ring(512),
			minClusterSize: 3,
			waitTimeout:    20 * time.Millisecond,
			onChange:       func() { changed <- struct{}{} },
		}

		require.False(t, n.Ready(), "must not be ready before the deadline")

		n.startReadinessDeadline()

		// Advance the fake clock past the deadline; Wait then lets the timer's
		// callback goroutine settle.
		time.Sleep(n.waitTimeout)
		synctest.Wait()

		require.True(t, n.Ready(), "must be ready after the deadline passes")
		require.Len(t, changed, 1, "deadline must fire onChange exactly once")
	})
}

// TestReconcileObserver verifies our wrapper: onChange fires on a participant
// change, the observer stays registered, and viewer churn is filtered out (the
// reason we wrap with ParticipantObserver).
func TestReconcileObserver(t *testing.T) {
	calls := 0
	obs := reconcileObserver(func() { calls++ })

	// A participant change fires onChange and keeps the observer registered.
	require.True(t, obs.NotifyPeersChanged(participants("a", "a")))
	require.Equal(t, 1, calls)

	// A viewer joining is not a participant change: onChange must not fire.
	withViewer := append(participants("a", "a"), peer.Peer{
		Name:  "v",
		Addr:  "v:80",
		State: peer.StateViewer,
	})
	require.True(t, obs.NotifyPeersChanged(withViewer))
	require.Equal(t, 1, calls)

	// A new participant joining fires onChange again.
	require.True(t, obs.NotifyPeersChanged(participants("a", "a", "b")))
	require.Equal(t, 2, calls)
}

// TestReconcileObserverNilOnChange verifies the observer tolerates a nil
// onChange without panicking.
func TestReconcileObserverNilOnChange(t *testing.T) {
	obs := reconcileObserver(nil)
	require.True(t, obs.NotifyPeersChanged(participants("a", "a")))
}

// TestStop verifies the graceful-shutdown path on a lone node: it announces
// departure, keeps running through the drain window, then leaves the cluster.
// Multi-node handover is covered by the integration test (item 14).
func TestStop(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	r, err := NewRingNode(RingConfig{
		Name:          "stop-node",
		AdvertiseAddr: lis.Addr().String(),
		Client:        NewGossipClient(),
		DrainTimeout:  50 * time.Millisecond,
	}, nil)
	require.NoError(t, err)

	route, h := r.Handler()
	srv := NewGossipServer(route, h)
	defer func() { _ = srv.Shutdown(context.Background()) }()
	go func() { _ = srv.Run(lis) }()

	require.NoError(t, r.join())
	require.NoError(t, r.setParticipant(context.Background()))

	start := time.Now()
	require.NoError(t, r.Stop(context.Background()))
	require.GreaterOrEqual(t, time.Since(start), r.drainTimeout,
		"Stop must keep running through the drain window before leaving")
}

// TestRingNodeMetrics verifies the cluster size and ring-ready gauges registered
// by NewRingNode reflect the live ring state.
func TestRingNodeMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()

	r, err := NewRingNode(RingConfig{
		Name:               "metrics-node",
		AdvertiseAddr:      "127.0.0.1:7946",
		Client:             &http.Client{},
		MinimumClusterSize: 3,
	}, reg)
	require.NoError(t, err)

	// Before reaching the minimum the ring is not ready and has no peers.
	require.Equal(t, 0.0, gaugeValue(t, reg, "sm_agent_cluster_ring_ready"))
	require.Equal(t, 0.0, gaugeValue(t, reg, "sm_agent_cluster_size"))

	// Reaching the minimum cluster size latches readiness.
	r.sharder.SetPeers(participants("metrics-node", "metrics-node", "b", "c"))
	r.updateReadyState()

	require.Equal(t, 1.0, gaugeValue(t, reg, "sm_agent_cluster_ring_ready"))
	require.Equal(t, 3.0, gaugeValue(t, reg, "sm_agent_cluster_size"))
}

// TestJoinResolveFailure verifies that a peer-discovery failure on join is
// counted and recovered by bootstrapping a single-node ring (fail-open).
func TestJoinResolveFailure(t *testing.T) {
	reg := prometheus.NewRegistry()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	r, err := NewRingNode(RingConfig{
		Name:          "join-fail-node",
		AdvertiseAddr: lis.Addr().String(),
		Client:        NewGossipClient(),
		Discover:      func() ([]string, error) { return nil, errors.New("discovery boom") },
	}, reg)
	require.NoError(t, err)

	route, h := r.Handler()
	srv := NewGossipServer(route, h)
	defer func() { _ = srv.Shutdown(context.Background()) }()
	go func() { _ = srv.Run(lis) }()

	// Discovery fails, but join falls back to a single-node bootstrap.
	require.NoError(t, r.join())
	require.Equal(t, 1.0, counterValue(t, reg, "sm_agent_cluster_peer_resolve_failures_total"))
}

// participants builds a Participant peer set, marking the peer named self as the
// local node.
func participants(self string, names ...string) []peer.Peer {
	ps := make([]peer.Peer, 0, len(names))
	for _, name := range names {
		ps = append(ps, peer.Peer{
			Name:  name,
			Addr:  name + ":80",
			Self:  name == self,
			State: peer.StateParticipant,
		})
	}
	return ps
}

// ringCluster returns one RingNode per name, each seeing the same peer set but
// with its own Self flag set, simulating every agent's local view.
func ringCluster(names ...string) map[string]*RingNode {
	cluster := make(map[string]*RingNode, len(names))
	for _, self := range names {
		s := shard.Ring(512)
		s.SetPeers(participants(self, names...))
		cluster[self] = &RingNode{sharder: s}
	}
	return cluster
}

func gaugeValue(t *testing.T, g prometheus.Gatherer, name string) float64 {
	t.Helper()

	mfs, err := g.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() == name {
			require.NotEmpty(t, mf.GetMetric())
			return mf.GetMetric()[0].GetGauge().GetValue()
		}
	}
	t.Fatalf("metric %q not found", name)
	return 0
}

func counterValue(t *testing.T, g prometheus.Gatherer, name string) float64 {
	t.Helper()

	mfs, err := g.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() == name {
			require.NotEmpty(t, mf.GetMetric())
			return mf.GetMetric()[0].GetCounter().GetValue()
		}
	}
	t.Fatalf("metric %q not found", name)
	return 0
}
