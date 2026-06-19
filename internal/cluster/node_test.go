package cluster

import (
	"testing"

	"github.com/grafana/ckit/peer"
	"github.com/grafana/ckit/shard"
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

// ringCluster returns one ringNode per name, each seeing the same peer set but
// with its own Self flag set, simulating every agent's local view.
func ringCluster(names ...string) map[string]*ringNode {
	cluster := make(map[string]*ringNode, len(names))
	for _, self := range names {
		s := shard.Ring(512)
		s.SetPeers(participants(self, names...))
		cluster[self] = &ringNode{sharder: s}
	}
	return cluster
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
		for i := 0; i < 5; i++ {
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
	n := &ringNode{sharder: shard.Ring(512)} // no peers set

	_, err := n.IsOwner(1)
	require.Error(t, err)
}
