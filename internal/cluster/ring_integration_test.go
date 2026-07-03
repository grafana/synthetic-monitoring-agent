package cluster

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
)

// These tests bring up real RingNodes gossiping over loopback and assert the
// RF=1 behaviours: disjoint+complete ownership, rebalance on join, and handover
// on leave. Convergence is timing-based, so they poll with require.Eventually
// and are skipped under -short.

const (
	convergeTimeout = 15 * time.Second
	convergeTick    = 100 * time.Millisecond
)

type testNode struct {
	node   *RingNode
	name   string
	addr   string
	srv    *GossipServer
	errc   chan error
	cancel context.CancelFunc
}

// startNode brings up a single gossip node: it serves the ckit handler on lis,
// then runs Start (join -> participant -> rejoin loop) in a goroutine under a
// context derived from parent. peers is the static discovery set (the other
// nodes' advertise addresses).
func startNode(t *testing.T, parent context.Context, name string, lis net.Listener, peers []string, minSize int) *testNode {
	t.Helper()

	node, err := NewRingNode(RingConfig{
		Name:               name,
		AdvertiseAddr:      lis.Addr().String(),
		Client:             NewGossipClient(),
		Discover:           func() ([]string, error) { return peers, nil },
		RejoinInterval:     200 * time.Millisecond,
		MinimumClusterSize: minSize,
		DrainTimeout:       50 * time.Millisecond,
	}, nil)
	require.NoError(t, err)

	route, h := node.Handler()
	srv := NewGossipServer(route, h)
	go func() { _ = srv.Run(lis) }()

	ctx, cancel := context.WithCancel(parent)
	errc := make(chan error, 1)
	go func() { errc <- node.Start(ctx, func() {}) }()

	tn := &testNode{node: node, name: name, addr: lis.Addr().String(), srv: srv, errc: errc, cancel: cancel}

	t.Cleanup(func() {
		cancel()
		_ = srv.Shutdown(context.Background())
		select {
		case err := <-errc:
			if err != nil && !isContextErr(err) {
				t.Errorf("node %s Start returned unexpected error: %v", name, err)
			}
		case <-time.After(time.Second):
		}
	})

	return tn
}

func isContextErr(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

// newLoopbackListeners creates n loopback listeners; their addresses are known
// up front so each node can be told about the others.
func newLoopbackListeners(t *testing.T, n int) []net.Listener {
	t.Helper()

	lis := make([]net.Listener, n)
	for i := range lis {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		lis[i] = l
	}
	return lis
}

// otherAddrs returns every listener address except the one at index self.
func otherAddrs(lis []net.Listener, self int) []string {
	addrs := make([]string, 0, len(lis)-1)
	for i, l := range lis {
		if i != self {
			addrs = append(addrs, l.Addr().String())
		}
	}
	return addrs
}

// awaitOwnership polls until every id has exactly one owner across nodes, then
// returns the converged id -> owner-name map. It never calls require inside the
// condition (testify runs it on a separate goroutine).
func awaitOwnership(t *testing.T, nodes []*testNode, ids []model.GlobalID) map[model.GlobalID]string {
	t.Helper()

	var converged map[model.GlobalID]string
	require.Eventually(t, func() bool {
		m := make(map[model.GlobalID]string, len(ids))
		for _, id := range ids {
			owners := 0
			owner := ""
			for _, n := range nodes {
				mine, err := n.node.IsOwner(id)
				if err != nil {
					return false
				}
				if mine {
					owners++
					owner = n.name
				}
			}
			if owners != 1 {
				return false
			}
			m[id] = owner
		}
		converged = m
		return true
	}, convergeTimeout, convergeTick, "checks did not converge to exactly one owner each")

	return converged
}

func ownerCounts(m map[model.GlobalID]string) map[string]int {
	counts := make(map[string]int)
	for _, owner := range m {
		counts[owner]++
	}
	return counts
}

func syntheticIDs(n int) []model.GlobalID {
	ids := make([]model.GlobalID, n)
	for i := range ids {
		ids[i] = model.GlobalID(i + 1)
	}
	return ids
}

// TestRingClusterOwnershipDisjoint verifies two gossiping nodes split a check
// set so that every check has exactly one owner and both nodes carry a share.
func TestRingClusterOwnershipDisjoint(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-node gossip convergence test; skipped under -short")
	}

	ctx := t.Context()
	lis := newLoopbackListeners(t, 2)
	nodes := []*testNode{
		startNode(t, ctx, "node-0", lis[0], otherAddrs(lis, 0), 1),
		startNode(t, ctx, "node-1", lis[1], otherAddrs(lis, 1), 1),
	}

	ids := syntheticIDs(200)
	owners := awaitOwnership(t, nodes, ids)

	counts := ownerCounts(owners)
	require.Greater(t, counts["node-0"], 0, "node-0 must own some checks")
	require.Greater(t, counts["node-1"], 0, "node-1 must own some checks")
	require.Equal(t, len(ids), counts["node-0"]+counts["node-1"], "every check owned exactly once")
}

// TestRingClusterRebalanceOnJoin verifies that a third node joining an existing
// two-node ring takes over a share of the checks (a real rebalance, not just
// additive growth).
func TestRingClusterRebalanceOnJoin(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-node gossip convergence test; skipped under -short")
	}

	ctx := t.Context()
	lis := newLoopbackListeners(t, 3)

	// Bring up the first two nodes and let them converge.
	nodes := []*testNode{
		startNode(t, ctx, "node-0", lis[0], []string{lis[1].Addr().String()}, 1),
		startNode(t, ctx, "node-1", lis[1], []string{lis[0].Addr().String()}, 1),
	}

	ids := syntheticIDs(300)
	before := awaitOwnership(t, nodes, ids)

	// Add a third node that discovers the first two; gossip propagates its
	// arrival to the whole ring.
	nodes = append(nodes, startNode(t, ctx, "node-2", lis[2],
		[]string{lis[0].Addr().String(), lis[1].Addr().String()}, 1))

	after := awaitOwnership(t, nodes, ids)

	counts := ownerCounts(after)
	require.Greater(t, counts["node-2"], 0, "the joined node must take ownership of some checks")
	require.Equal(t, len(ids), counts["node-0"]+counts["node-1"]+counts["node-2"], "every check owned exactly once")

	moved := 0
	for _, id := range ids {
		if before[id] != after[id] {
			moved++
		}
	}
	require.Greater(t, moved, 0, "joining must rebalance some checks onto the new node")
}

// TestRingClusterHandoverOnLeave verifies that when a node leaves gracefully,
// the checks it owned are picked up by the surviving nodes.
func TestRingClusterHandoverOnLeave(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-node gossip convergence test; skipped under -short")
	}

	ctx := t.Context()
	lis := newLoopbackListeners(t, 3)
	nodes := []*testNode{
		startNode(t, ctx, "node-0", lis[0], otherAddrs(lis, 0), 1),
		startNode(t, ctx, "node-1", lis[1], otherAddrs(lis, 1), 1),
		startNode(t, ctx, "node-2", lis[2], otherAddrs(lis, 2), 1),
	}

	ids := syntheticIDs(300)
	before := awaitOwnership(t, nodes, ids)

	leaving := nodes[2]
	require.Greater(t, ownerCounts(before)[leaving.name], 0, "leaving node should own checks before departure")

	// Graceful leave: stop the rejoin loop (cancel) so it cannot re-add itself,
	// then drain + leave the cluster.
	leaving.cancel()
	require.NoError(t, leaving.node.Stop(context.Background()))

	survivors := nodes[:2]
	after := awaitOwnership(t, survivors, ids)

	counts := ownerCounts(after)
	require.Equal(t, len(ids), counts["node-0"]+counts["node-1"], "survivors must own every check exactly once")

	// Every check the departed node owned must now belong to a survivor.
	for _, id := range ids {
		if before[id] == leaving.name {
			require.Contains(t, []string{"node-0", "node-1"}, after[id],
				"check %d owned by departed node must hand over to a survivor", id)
		}
	}
}
