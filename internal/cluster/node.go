package cluster

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/grafana/ckit"
	"github.com/grafana/ckit/peer"
	"github.com/grafana/ckit/shard"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
)

// DefaultRejoinInterval is used when RingConfig.RejoinInterval is zero.
const DefaultRejoinInterval = 60 * time.Second

// DefaultMinimumSizeWaitTimeout is used by NewRingNode when
// RingConfig.MinimumSizeWaitTimeout is zero. After it elapses without reaching
// the minimum cluster size, the node becomes ready anyway (fail-open).
const DefaultMinimumSizeWaitTimeout = 60 * time.Second

// DefaultDrainTimeout is used by Stop when RingConfig.DrainTimeout is zero. It
// is how long the node stays in the cluster as Terminating after announcing its
// departure, giving surviving peers time to observe the change and take over its
// checks before it leaves. Keep it well under the deployment's termination grace
// period so the node stops cleanly before being force-killed.
const DefaultDrainTimeout = 10 * time.Second

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

// metricNamespace is the Prometheus namespace for cluster metrics, matching the
// rest of the agent.
const metricNamespace = "sm_agent"

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
	logger         zerolog.Logger
	node           *ckit.Node
	sharder        shard.Sharder
	onChange       func()
	discover       DiscoverFn
	rejoinInterval time.Duration

	minClusterSize int
	waitTimeout    time.Duration
	drainTimeout   time.Duration

	mu         sync.Mutex
	readyState readyState
	deadline   *time.Timer

	metrics struct {
		resolveFailures prometheus.Counter
		joinFailures    prometheus.Counter
	}
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
	// Logger records join/rejoin failures. The zero value is a silent no-op.
	Logger zerolog.Logger
	// Discover resolves the peers to join. It is called by Join at startup and
	// re-invoked on every RejoinInterval.
	Discover DiscoverFn
	// RejoinInterval is how often the node re-resolves peers and re-joins, picking up
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
	// DrainTimeout is how long Stop stays in the cluster as Terminating after
	// announcing departure, giving peers time to take over before it leaves. Zero
	// uses DefaultDrainTimeout.
	DrainTimeout time.Duration
}

// NewRingNode builds a gossip-backed RingNode. The returned node is not yet a cluster
// member: the caller must Start it and transition it to Participant.
func NewRingNode(cfg RingConfig, registerer prometheus.Registerer) (*RingNode, error) {
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

	drainTimeout := cfg.DrainTimeout
	if drainTimeout <= 0 {
		drainTimeout = DefaultDrainTimeout
	}

	r := &RingNode{
		logger:         cfg.Logger,
		node:           node,
		sharder:        sharder,
		discover:       cfg.Discover,
		rejoinInterval: rejoinInterval,
		minClusterSize: cfg.MinimumClusterSize,
		waitTimeout:    waitTimeout,
		drainTimeout:   drainTimeout,
		readyState:     stateNotReady,
	}

	if registerer == nil {
		registerer = prometheus.NewRegistry()
	}
	if err := r.registerMetrics(registerer); err != nil {
		return nil, err
	}

	return r, nil
}

// Handler returns the route and HTTP handler for gossip traffic.
func (r *RingNode) Handler() (string, http.Handler) { return r.node.Handler() }

// Metrics returns the ckit node's Prometheus collector.
func (r *RingNode) Metrics() prometheus.Collector { return r.node.Metrics() }

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

// Start registers onChange (invoked whenever the set of participant peers
// changes; may be nil), joins the cluster, becomes a participant (eligible to
// own checks), and then runs the periodic rejoin loop until ctx is cancelled. It
// blocks; run it under errgroup.Go. Serve Handler() before calling Start.
func (r *RingNode) Start(ctx context.Context, onChange func()) error {
	r.logger.Info().Msg("starting cluster node")
	r.onChange = onChange
	// Subscribe to participant-set changes:
	// ckit invokes handleMembershipChange on every join/leave.
	r.node.Observe(reconcileObserver(r.handleMembershipChange))

	if err := r.join(); err != nil {
		return err
	}
	if err := r.setParticipant(ctx); err != nil {
		return err
	}
	return r.rejoinLoop(ctx)
}

// join resolves peers via the configured DiscoverFn and joins the cluster. If
// discovery fails or returns no peers, the node bootstraps a single-node
// cluster; the rejoin loop folds in peers once discovery succeeds, since ckit's
// Start is additive. Failures are logged and counted but not propagated: the
// bootstrap fallback keeps the agent running (fail-open), at the cost of briefly
// owning every check until it rejoins.
func (r *RingNode) join() error {
	peers, err := r.resolvePeers()
	if err != nil {
		r.metrics.resolveFailures.Inc()
		r.logger.Warn().Err(err).Msg("peer discovery failed; bootstrapping single-node ring")

		return r.node.Start(nil)
	}

	if len(peers) == 0 {
		r.logger.Info().Msg("no peers discovered; bootstrapping single-node ring")

		return r.node.Start(nil)
	}

	if err := r.node.Start(peers); err != nil {
		r.metrics.joinFailures.Inc()
		r.logger.Warn().Err(err).Strs("peers", peers).Msg(
			"joining discovered peers failed; bootstrapping single-node ring and retrying",
		)

		return r.node.Start(nil)
	}

	r.logger.Info().Int("peers", len(peers)).Msg("joined cluster")

	return nil
}

// rejoinLoop periodically re-resolves peers and re-joins so the ring picks up
// scale-ups and restarted peers. It blocks until ctx is cancelled.
func (r *RingNode) rejoinLoop(ctx context.Context) error {
	ticker := time.NewTicker(r.rejoinInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			peers, err := r.resolvePeers()
			if err != nil {
				r.metrics.resolveFailures.Inc()
				r.logger.Warn().Err(err).Msg("peer discovery failed during rejoin; retrying next interval")

				continue
			}

			if len(peers) == 0 {
				continue
			}

			// Start is additive; transient errors are retried on the next tick.
			if err := r.node.Start(peers); err != nil {
				r.metrics.joinFailures.Inc()
				r.logger.Warn().Err(err).Msg("rejoin failed; retrying next interval")
			}
		}
	}
}

func (r *RingNode) resolvePeers() ([]string, error) {
	if r.discover == nil {
		return nil, nil
	}
	return r.discover()
}

// setParticipant transitions the node to the Participant state, making it
// eligible to own checks. It arms the readiness deadline: convergence only
// matters once the node can own checks.
func (r *RingNode) setParticipant(ctx context.Context) error {
	if err := r.node.ChangeState(ctx, peer.StateParticipant); err != nil {
		return err
	}
	r.startReadinessDeadline()
	return nil
}

// setTerminating transitions the node to the Terminating state so surviving
// peers take over its checks before it leaves.
func (r *RingNode) setTerminating(ctx context.Context) error {
	return r.node.ChangeState(ctx, peer.StateTerminating)
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

// Stop gracefully removes the node from the cluster on shutdown: it first drains
// (announces departure and waits the drain window so surviving peers take over)
// and then leaves the cluster.
//
// It is best-effort: a failed drain does not skip leaving.
// ctx bounds the drain window (whichever of ctx or DrainTimeout elapses first).
func (r *RingNode) Stop(ctx context.Context) error {
	r.logger.Info().Msg("stopping cluster node")

	if err := r.drain(ctx); err != nil {
		r.logger.Warn().Err(err).Msg("failed to drain cluster node")
	}

	if r.deadline != nil {
		r.deadline.Stop()
	}

	if err := r.node.Stop(); err != nil {
		r.logger.Warn().Err(err).Msg("failed to stop cluster node")
		return fmt.Errorf("stopping cluster node: %w", err)
	}

	return nil
}

// drain announces departure (Terminating, which OpReadWrite excludes, so
// surviving peers' observers fire and take over this node's checks) and waits
// the drain window so that takeover can happen while this node is still a known
// cluster member, before it leaves.
func (r *RingNode) drain(ctx context.Context) error {
	if err := r.setTerminating(ctx); err != nil {
		return fmt.Errorf("transitioning to terminating state: %w", err)
	}

	select {
	case <-time.After(r.drainTimeout):
	case <-ctx.Done():
	}

	return nil
}

func (r *RingNode) registerMetrics(reg prometheus.Registerer) error {
	if err := reg.Register(r.node.Metrics()); err != nil {
		return err
	}

	clusterSize := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Subsystem: "cluster",
		Name:      "size",
		Help:      "Number of participant peers in the ring, including this node.",
	}, func() float64 {
		return float64(len(r.sharder.Peers()))
	})
	if err := reg.Register(clusterSize); err != nil {
		return err
	}

	ringReady := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Subsystem: "cluster",
		Name:      "ring_ready",
		Help:      "Whether the ring has converged enough to trust ownership (1) or not (0).",
	}, func() float64 {
		if r.Ready() {
			return 1
		}
		return 0
	})
	if err := reg.Register(ringReady); err != nil {
		return err
	}

	r.metrics.resolveFailures = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricNamespace,
		Subsystem: "cluster",
		Name:      "peer_resolve_failures_total",
		Help:      "Number of peer discovery/resolution failures on join and rejoin.",
	})
	if err := reg.Register(r.metrics.resolveFailures); err != nil {
		return err
	}

	r.metrics.joinFailures = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricNamespace,
		Subsystem: "cluster",
		Name:      "join_failures_total",
		Help:      "Number of failures joining discovered peers on join and rejoin.",
	})
	return reg.Register(r.metrics.joinFailures)
}
