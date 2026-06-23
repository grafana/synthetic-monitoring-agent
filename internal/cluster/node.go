package cluster

import (
	"context"
	"net/http"

	"github.com/grafana/ckit"
	"github.com/grafana/ckit/peer"
	"github.com/grafana/ckit/shard"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
)

// tokensPerNode defines how many tokens each node is given on the consistent-hash
// ring. All nodes must use the same value, otherwise they build different views
// of the ring and assign checks inconsistently.
//
// 512 strikes a good balance between distribution accuracy and memory: a
// 1,000-node cluster needs ~12MB for the ring. Lower values distribute keys
// poorly.
const tokensPerNode = 512

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

// RingNode is the gossip-backed implementation of Node. It wraps a ckit.Node and a
// consistent-hash sharder, and notifies the rest of the agent (via onChange)
// whenever cluster membership changes.
type RingNode struct {
	node     *ckit.Node
	sharder  shard.Sharder
	onChange func()
}

var _ Node = (*RingNode)(nil)

// RingConfig configures a Ring.
type RingConfig struct {
	// Name is this node's unique, stable identity in the cluster.
	Name string
	// AdvertiseAddr is the host:port other nodes use to reach this one.
	AdvertiseAddr string
	// Label prevents distinct clusters from accidentally merging: nodes only
	// join peers sharing the same label.
	Label string
	// Client is the HTTP/2 client used for gossip transport (built by the
	// caller; see item 7).
	Client *http.Client
	// OnChange is invoked whenever the set of participant peers changes.
	OnChange func()
}

// NewRingNode builds a gossip-backed RingNode. The returned node is not yet a cluster
// member: the caller must Start it and transition it to Participant.
func NewRingNode(cfg RingConfig) (*RingNode, error) {
	sharder := shard.Ring(tokensPerNode)

	node, err := ckit.NewNode(cfg.Client, ckit.Config{
		Name:          cfg.Name,
		AdvertiseAddr: cfg.AdvertiseAddr,
		Label:         cfg.Label,
		Sharder:       sharder,
		// TODO:
		// Log is left nil: ckit falls back to a nop logger.
		// Wire ckit's go-kit logger to the agent's zerolog.
	})
	if err != nil {
		return nil, err
	}

	r := &RingNode{node: node, sharder: sharder, onChange: cfg.OnChange}
	node.Observe(reconcileObserver(r.onChange))

	return r, nil
}

// reconcileObserver builds a ckit observer that invokes onChange on every change
// to the set of participant peers (viewer churn is filtered out by ParticipantObserver).
func reconcileObserver(onChange func()) ckit.Observer {
	return ckit.ParticipantObserver(ckit.FuncObserver(func(_ []peer.Peer) bool {
		if onChange != nil {
			onChange()
		}
		// Stay registered for the node's lifetime; Stop() tears everything down.
		return true
	}))
}

// IsOwner reports whether the local node owns the check.
func (r *RingNode) IsOwner(globalID model.GlobalID) (bool, error) {
	owners, err := r.sharder.Lookup(keyOf(globalID), 1, shard.OpReadWrite)
	if err != nil {
		return false, err
	}
	return owners[0].Self, nil
}

// Ready reports whether the ring has converged enough to trust ownership.
// TODO: Set min-cluster-size gate. For now it is fail-open.
func (r *RingNode) Ready() bool { return true }

// Start joins the cluster, attempting to connect to the given peers.
func (r *RingNode) Start(peers []string) error { return r.node.Start(peers) }

// SetParticipant transitions the node to the Participant state, making it
// eligible to own checks.
func (r *RingNode) SetParticipant(ctx context.Context) error {
	return r.node.ChangeState(ctx, peer.StateParticipant)
}

// SetTerminating transitions the node to the Terminating state so surviving
// peers take over its checks before it leaves.
func (r *RingNode) SetTerminating(ctx context.Context) error {
	return r.node.ChangeState(ctx, peer.StateTerminating)
}

// Stop removes the node from the cluster.
func (r *RingNode) Stop() error { return r.node.Stop() }

// Handler returns the route and HTTP handler for gossip traffic.
func (r *RingNode) Handler() (string, http.Handler) { return r.node.Handler() }

// Metrics returns the ckit node's Prometheus collector.
func (r *RingNode) Metrics() prometheus.Collector { return r.node.Metrics() }
