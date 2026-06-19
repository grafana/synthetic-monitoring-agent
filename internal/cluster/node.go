package cluster

import (
	"github.com/grafana/ckit/shard"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
)

// Node is an agent's view of itself as a member of the cluster. Each agent holds
// one Node and uses it to decide which checks it should run.
type Node interface {
	// IsOwner reports whether this agent owns (should run) the check under the
	// configured replication factor (RF=1).
	IsOwner(globalID model.GlobalID) (bool, error)
	// Ready reports whether this node's view of the cluster has converged enough
	// to trust ownership decisions.
	Ready() bool
}

// monoNode is the default when clustering is disabled: a single-node cluster
// that owns every check, preserving the agent's pre-clustering behavior.
type monoNode struct{}

func NewMono() Node { return monoNode{} }

func (monoNode) IsOwner(model.GlobalID) (bool, error) { return true, nil }

func (monoNode) Ready() bool { return true }

// ringNode is the gossip-backed implementation of Node.
type ringNode struct {
	sharder shard.Sharder
}

// IsOwner reports whether the local node owns (should run) the check.
func (n *ringNode) IsOwner(globalID model.GlobalID) (bool, error) {
	owners, err := n.sharder.Lookup(keyOf(globalID), 1, shard.OpReadWrite)
	if err != nil {
		return false, err
	}
	return owners[0].Self, nil
}
