package cluster

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/grafana/ckit"
	"github.com/grafana/ckit/peer"
	"github.com/grafana/ckit/shard"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
)

// DefaultRejoinInterval is used by Run when RingConfig.RejoinInterval is zero.
const DefaultRejoinInterval = 60 * time.Second

// DefaultMinimumSizeWaitTimeout is used by NewRingNode when
// RingConfig.MinimumSizeWaitTimeout is zero. After it elapses without reaching
// the minimum cluster size, the node becomes ready anyway (fail-open).
const DefaultMinimumSizeWaitTimeout = 60 * time.Second

// readyState is the convergence state machine consulted by Ready. It latches:
// once stateReady or stateDeadlinePassed is reached it never returns to
// stateNotReady, so a transient dip below the minimum cluster size does not
// stop steady-state reconciliation.
type readyState int

const (
	// stateNotReady is the initial state: the ring has not yet reached the
	// minimum cluster size and the wait-timeout has not elapsed, so ownership is
	// not trusted and checks are buffered.
	stateNotReady readyState = iota
	// stateReady means the ring reached the minimum cluster size; ownership is
	// trusted.
	stateReady
	// stateDeadlinePassed means the minimum was never reached but the
	// wait-timeout elapsed, so the node trusts ownership anyway (fail-open).
	stateDeadlinePassed
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
	node           *ckit.Node
	sharder        shard.Sharder
	onChange       func()
	discover       DiscoverFn
	rejoinInterval time.Duration

	minClusterSize int
	waitTimeout    time.Duration

	mu         sync.Mutex
	readyState readyState
	deadline   *time.Timer
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
	// Discover resolves the peers to join. It is called by Join at startup and
	// re-invoked by Run on every RejoinInterval.
	Discover DiscoverFn
	// RejoinInterval is how often Run re-resolves peers and re-joins, picking up
	// scale-ups and restarted peers. Zero uses DefaultRejoinInterval.
	RejoinInterval time.Duration
	// MinimumClusterSize is the number of peers (including this node) the ring
	// must reach before Ready reports true. Zero or one makes the node ready
	// immediately (fail-open: a lone agent runs everything).
	MinimumClusterSize int
	// MinimumSizeWaitTimeout bounds how long the node waits to reach
	// MinimumClusterSize: once it elapses the node becomes ready anyway. Zero
	// uses DefaultMinimumSizeWaitTimeout.
	MinimumSizeWaitTimeout time.Duration
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

	rejoinInterval := cfg.RejoinInterval
	if rejoinInterval <= 0 {
		rejoinInterval = DefaultRejoinInterval
	}

	waitTimeout := cfg.MinimumSizeWaitTimeout
	if waitTimeout <= 0 {
		waitTimeout = DefaultMinimumSizeWaitTimeout
	}

	r := &RingNode{
		node:           node,
		sharder:        sharder,
		onChange:       cfg.OnChange,
		discover:       cfg.Discover,
		rejoinInterval: rejoinInterval,
		minClusterSize: cfg.MinimumClusterSize,
		waitTimeout:    waitTimeout,
		readyState:     stateNotReady,
	}
	node.Observe(reconcileObserver(r.handleMembershipChange))

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

// handleMembershipChange runs on every participant-set change. It refreshes the
// readiness state first so a reconcile triggered by onChange observes the new
// Ready() value, then notifies the rest of the agent. onChange is invoked
// without r.mu held: it runs on ckit's notifier goroutine and feeds the
// non-blocking RequestReconcile.
func (r *RingNode) handleMembershipChange() {
	r.updateReadyState()
	if r.onChange != nil {
		r.onChange()
	}
}

// updateReadyState latches the node ready once the sharder reports at least
// MinimumClusterSize peers. It is a no-op once already latched or when the
// minimum is trivially satisfied.
func (r *RingNode) updateReadyState() {
	if r.minClusterSize <= 1 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.readyState != stateNotReady {
		return
	}
	if len(r.sharder.Peers()) >= r.minClusterSize {
		r.readyState = stateReady
	}
}

// startReadinessDeadline arms the fail-open timer: if MinimumClusterSize is
// never reached within waitTimeout, the node becomes ready anyway and a single
// onChange fires so checks buffered before convergence start. It is a no-op when
// the minimum is trivially satisfied.
func (r *RingNode) startReadinessDeadline() {
	if r.minClusterSize <= 1 || r.waitTimeout <= 0 {
		return
	}

	r.deadline = time.AfterFunc(r.waitTimeout, func() {
		r.mu.Lock()
		if r.readyState == stateNotReady {
			r.readyState = stateDeadlinePassed
		}
		r.mu.Unlock()

		if r.onChange != nil {
			r.onChange()
		}
	})
}

// IsOwner reports whether the local node owns the check.
func (r *RingNode) IsOwner(globalID model.GlobalID) (bool, error) {
	owners, err := r.sharder.Lookup(keyOf(globalID), 1, shard.OpReadWrite)
	if err != nil {
		return false, err
	}
	return owners[0].Self, nil
}

// Ready reports whether the ring has converged enough to trust ownership. With
// MinimumClusterSize <= 1 it is always true (a lone agent runs everything);
// otherwise it is true once the node has latched ready, either by reaching the
// minimum cluster size or by the wait-timeout deadline passing (fail-open).
func (r *RingNode) Ready() bool {
	if r.minClusterSize <= 1 {
		return true
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.readyState != stateNotReady
}

// Join resolves peers via the configured DiscoverFn and joins the cluster. If
// discovery fails or returns no peers, the node bootstraps a single-node
// cluster; Run folds in peers once discovery succeeds, since ckit's Start is
// additive.
//
// TODO: log discovery/join failures once a logger is wired into RingNode (see
// the Log TODO in NewRingNode); they are currently recovered silently by the
// bootstrap fallback and Run's retry.
func (r *RingNode) Join() error {
	peers, err := r.resolvePeers()
	if err != nil || len(peers) == 0 {
		return r.node.Start(nil)
	}
	if err := r.node.Start(peers); err != nil {
		// Joining the discovered peers failed; bootstrap solo and let Run retry.
		return r.node.Start(nil)
	}
	return nil
}

// Run periodically re-resolves peers and re-joins so the ring picks up
// scale-ups and restarted peers. It blocks until ctx is cancelled.
func (r *RingNode) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.rejoinInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			peers, err := r.resolvePeers()
			if err != nil || len(peers) == 0 {
				continue
			}
			// Start is additive; transient errors are retried on the next tick.
			_ = r.node.Start(peers)
		}
	}
}

func (r *RingNode) resolvePeers() ([]string, error) {
	if r.discover == nil {
		return nil, nil
	}
	return r.discover()
}

// SetParticipant transitions the node to the Participant state, making it
// eligible to own checks. It arms the readiness deadline: convergence only
// matters once the node can own checks.
func (r *RingNode) SetParticipant(ctx context.Context) error {
	if err := r.node.ChangeState(ctx, peer.StateParticipant); err != nil {
		return err
	}
	r.startReadinessDeadline()
	return nil
}

// SetTerminating transitions the node to the Terminating state so surviving
// peers take over its checks before it leaves.
func (r *RingNode) SetTerminating(ctx context.Context) error {
	return r.node.ChangeState(ctx, peer.StateTerminating)
}

// Stop removes the node from the cluster.
func (r *RingNode) Stop() error {
	if r.deadline != nil {
		r.deadline.Stop()
	}
	return r.node.Stop()
}

// Handler returns the route and HTTP handler for gossip traffic.
func (r *RingNode) Handler() (string, http.Handler) { return r.node.Handler() }

// Metrics returns the ckit node's Prometheus collector.
func (r *RingNode) Metrics() prometheus.Collector { return r.node.Metrics() }
