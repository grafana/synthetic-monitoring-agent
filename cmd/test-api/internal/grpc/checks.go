package grpc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gogo/status"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
)

type ChecksDb interface {
	FindProbeByID(ctx context.Context, id int64) (*sm.Probe, error)
	ListChecksForProbe(ctx context.Context, id int64) ([]sm.Check, error)
}

type ChecksServer struct {
	logger      zerolog.Logger
	db          ChecksDb
	probesMutex sync.Mutex
	probes      map[int64]probeController
}

type ChecksServerOpts struct {
	Logger zerolog.Logger
	Db     ChecksDb
}

func NewChecksServer(opts ChecksServerOpts) (*ChecksServer, error) {
	return &ChecksServer{
		logger: opts.Logger,
		db:     opts.Db,
		probes: make(map[int64]probeController),
	}, nil
}

func (s *ChecksServer) Run(ctx context.Context) error {
	// This is a dummy placeholder in case updating checks / probes /
	// tenants is needed at some point. The idea is that the ChecksServer
	// would monitor a channel with the updates and it would communicate
	// them to the appropriate probes.
	//
	// The code below already sort of keeps track of what's running where.
	<-ctx.Done()
	return nil
}

func (s *ChecksServer) RegisterProbe(ctx context.Context, _ *sm.ProbeInfo) (*sm.RegisterProbeResult, error) {
	probeID, found := probeIdFromContext(ctx)
	if !found {
		// This should never happen, the unary handler should have
		// added the probe ID to the context.
		return &sm.RegisterProbeResult{
			Status: sm.Status{
				Code:    sm.StatusCode_NOT_AUTHORIZED,
				Message: "cannot get probe ID",
			},
		}, nil
	}

	s.acquireProbesMutex(probeID)
	defer s.releaseProbesMutex(probeID)

	if _, found := s.probes[probeID]; found {
		// there's an existing probe with this ID
		return &sm.RegisterProbeResult{
			Status: sm.Status{
				Code:    sm.StatusCode_ALREADY_EXISTS,
				Message: "probe already exists",
			},
		}, nil
	}

	ready := make(chan readySignal)

	s.probes[probeID] = probeController{
		ch:           make(chan sm.Changes, probeUpdatesQueueLenght),
		adHocCheckCh: make(chan sm.AdHocCheck, probeUpdatesQueueLenght),
		restart:      make(chan restartSignal, probeUpdatesQueueLenght),
		ready:        ready,
		probeGoneCh:  make(chan probeGoneSignal),
		cleanupCh:    make(chan cleanupSignal),
	}

	go s.waitForProbe(probeID, probeRegistrationTimeout, ready)

	probe, err := s.db.FindProbeByID(ctx, probeID)
	if err != nil {
		return &sm.RegisterProbeResult{
			Status: sm.Status{
				Code:    sm.StatusCode_INTERNAL_ERROR,
				Message: "internal error retrieving probe",
			},
		}, nil
	}

	return &sm.RegisterProbeResult{Probe: *probe}, nil
}

func (s *ChecksServer) GetChanges(currentState *sm.ProbeState, stream sm.Checks_GetChangesServer) error {
	probeID, found := probeIdFromContext(stream.Context())
	if !found {
		return errors.New("invalid probe authorization")
	}

	defer s.deactivateProbe(probeID)

	updates, restart, probeGoneCh, err := s.activateProbe(probeID)
	if err != nil {
		return fmt.Errorf("activating probe %d: %w", probeID, err)
	}

	// Load all the existing checks from the database and send them to client

	existingChecks, err := s.sendInitialChanges(currentState, stream, probeID)
	if err != nil {
		return err
	}

	for {
		select {
		case up, ok := <-updates:
			// If the updates channel was closed, ok will be false. We should never hit
			// this, because place where we close the channel is in the deactivateProbe
			// call that happens when exiting this function.
			if !ok {
				continue
			}

			_, err := s.processUpdate(up, probeID, existingChecks, stream)
			if err != nil {
				return err
			}

		case <-restart:
			return status.Error(codes.Aborted, "operation aborted")

		case <-stream.Context().Done():
			return nil

		case <-probeGoneCh:
			// Probe is gone, stop.
			return status.Error(codes.Aborted, "operation aborted")
		}
	}
}

func (s *ChecksServer) Ping(ctx context.Context, req *sm.PingRequest) (*sm.PongResponse, error) {
	if _, found := probeIdFromContext(ctx); !found {
		return nil, errors.New("probe not found")
	}

	return &sm.PongResponse{Sequence: req.Sequence}, nil
}

const (
	probeUpdatesQueueLenght  = 128
	probeRegistrationTimeout = 1 * time.Second
)

type readySignal struct{}

type restartSignal struct{}

type probeGoneSignal struct{}

type cleanupSignal struct{}

type probeController struct {
	ch           chan sm.Changes
	adHocCheckCh chan sm.AdHocCheck
	restart      chan restartSignal
	ready        chan readySignal
	probeGoneCh  chan probeGoneSignal
	cleanupCh    chan cleanupSignal
}

type checksSet map[int64]struct{}

func (s *ChecksServer) acquireProbesMutex(id int64) {
	s.probesMutex.Lock()
}

func (s *ChecksServer) releaseProbesMutex(id int64) {
	s.probesMutex.Unlock()
}

func (s *ChecksServer) waitForProbe(id int64, timeout time.Duration, ready <-chan readySignal) {
	// Wait for the specified timeout for the probe to
	// ask for changes (handled in GetChanges); if the probe does
	// not show up, consider it dead and deactivate it.
	//
	// This handles a case where the probe managed to register
	// itself but didn't proceed thru to get changes.
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		s.logger.Warn().Int64("probe_id", id).Msg("timed out waiting for probe")
		s.deactivateProbe(id)

	case <-ready:
		s.logger.Info().Int64("probe_id", id).Msg("probe ready")
	}
}

// activateProbe searches for an existing probe and marks it as ready.
func (s *ChecksServer) activateProbe(id int64) (chan sm.Changes, chan restartSignal, chan probeGoneSignal, error) {
	s.acquireProbesMutex(id)
	defer s.releaseProbesMutex(id)

	updates, found := s.probes[id]
	if !found {
		return nil, nil, nil, fmt.Errorf("unknown probe")
	}

	close(updates.ready)

	return updates.ch, updates.restart, updates.probeGoneCh, nil
}

// deactivateProbe searches for an active probe and _removes_ it from the active list.
func (s *ChecksServer) deactivateProbe(id int64) {
	s.acquireProbesMutex(id)
	updates, found := s.probes[id]
	if found {
		delete(s.probes, id)
	}
	s.releaseProbesMutex(id)

	if !found {
		return
	}

DRAIN:
	for {
		select {
		case <-updates.ch:

		case <-updates.restart:

		default:
			break DRAIN
		}
	}

	close(updates.ch)
	close(updates.restart)
	close(updates.cleanupCh)
}

func (s *ChecksServer) processUpdate(update sm.Changes, probeID int64, existingChecks checksSet, stream sm.Checks_GetChangesServer) (bool, error) {
	checks := make([]sm.CheckChange, 0, len(update.Checks))

	for _, checkChange := range update.Checks {
		newChange := filterUpdate(probeID, checkChange, existingChecks)
		if newChange == nil {
			continue
		}

		checks = append(checks, *newChange)

		switch newChange.Operation {
		case sm.CheckOperation_CHECK_ADD:
			existingChecks[newChange.Check.Id] = struct{}{}

		case sm.CheckOperation_CHECK_UPDATE:
			// updates got converted to adds (handled above),
			// deletes (handle below), or already exist in this map
			// (don't do anything).

		case sm.CheckOperation_CHECK_DELETE:
			delete(existingChecks, newChange.Check.Id)
		}
	}

	update.Checks = checks

	if len(update.Checks) == 0 && len(update.Tenants) == 0 {
		return false, nil
	}

	// we have something that should be processed by this probe
	if err := stream.Send(&update); err != nil {
		return false, err
	}

	return true, nil
}

func filterUpdate(probeID int64, up sm.CheckChange, existingChecks checksSet) *sm.CheckChange {
	// There's an interesting problem here.
	//
	// For adds we can simply look at the check to see if _this_ probe is
	// in the list of probes where the check should run and send the
	// change to that probe.
	//
	// For deletes, the change operation only contains the check's ID
	// without other details, so we need to keep track of which checks are
	// running on this probe to now if the delete operation should be sent
	// there or not.
	//
	// For updates, if the probe is being added to the list, sending the
	// change operation would cause the check to start running on that
	// probe, but for removals, if we simply look at the list of probes in
	// the check itself, we would not send the change to a probe where the
	// check is already running.
	//
	// id | listed | existing | enabled | operation | result
	// ---+--------+----------+---------+-----------+--------
	//  1 | false  | false    | false   | ADD       | SKIP
	//  2 | false  | false    | false   | DELETE    | SKIP
	//  3 | false  | false    | false   | UPDATE    | SKIP
	//  4 | false  | false    | true    | ADD       | SKIP
	//  5 | false  | false    | true    | DELETE    | SKIP
	//  6 | false  | false    | true    | UPDATE    | SKIP
	//  7 | false  | true     | false   | ADD       | DELETE
	//  8 | false  | true     | true    | ADD       | DELETE
	//  9 | false  | true     | false   | DELETE    | DELETE
	// 10 | false  | true     | false   | UPDATE    | DELETE
	// 11 | false  | true     | true    | DELETE    | DELETE
	// 12 | false  | true     | true    | UPDATE    | DELETE
	// 13 | true   | false    | false   | ADD       | SKIP
	// 14 | true   | false    | false   | DELETE    | SKIP
	// 15 | true   | false    | false   | UPDATE    | SKIP
	// 16 | true   | false    | true    | ADD       | ADD
	// 17 | true   | false    | true    | DELETE    | SKIP
	// 18 | true   | false    | true    | UPDATE    | ADD
	// 19 | true   | true     | false   | ADD       | SKIP -> DELETE
	// 20 | true   | true     | false   | DELETE    | DELETE
	// 21 | true   | true     | false   | UPDATE    | DELETE
	// 22 | true   | true     | true    | ADD       | ADD?
	// 23 | true   | true     | true    | DELETE    | DELETE
	// 24 | true   | true     | true    | UPDATE    | UPDATE

	for _, newProbeID := range up.Check.Probes {
		if newProbeID != probeID {
			continue
		}

		if _, found := existingChecks[up.Check.Id]; found {
			// the probe did already know about this check

			if up.Check.Enabled {
				// pass-thru and let the probe handle the operation
				return &up
			}

			// if the check is being *disabled*, the
			// operation needs to be converted to a delete
			return &sm.CheckChange{
				Operation: sm.CheckOperation_CHECK_DELETE,
				Check:     sm.Check{Id: up.Check.Id},
			}
		}

		// the probe did not know about this check before
		if !up.Check.Enabled {
			return nil // SKIP
		}

		switch up.Operation {
		case sm.CheckOperation_CHECK_DELETE:
			return nil // SKIP

		case sm.CheckOperation_CHECK_ADD:
			return &up

		case sm.CheckOperation_CHECK_UPDATE:
			// updates for listed probes get converted to add operations
			return &sm.CheckChange{
				Operation: sm.CheckOperation_CHECK_ADD,
				Check:     up.Check,
			}

		default:
			panic("unhandled operation")
		}
	}

	// the probe was *not* found, either because it's not listed (ADD,
	// DELETE), or because it's being removed (UPDATE)

	if _, found := existingChecks[up.Check.Id]; !found {
		return nil // SKIP
	}

	switch up.Operation {
	case sm.CheckOperation_CHECK_ADD:
		// the check is being added, it is NOT meant for this
		// probe, but it DOES exist in the probe: transform it
		// to a DELETE
		return &sm.CheckChange{
			Operation: sm.CheckOperation_CHECK_DELETE,
			Check:     sm.Check{Id: up.Check.Id},
		}

	case sm.CheckOperation_CHECK_DELETE:
		return &up

	case sm.CheckOperation_CHECK_UPDATE:
		// updates and deletes for unlisted probes get converted to delete operations
		return &sm.CheckChange{
			Operation: sm.CheckOperation_CHECK_DELETE,
			Check:     sm.Check{Id: up.Check.Id},
		}

	default:
		panic("unhandled operation")
	}
}

// sendInitialChanges transfers the initial set of changes for the
// specified probe and returns the set of changes that were sent to the
// probe.
func (s *ChecksServer) sendInitialChanges(currentState *sm.ProbeState, stream sm.Checks_GetChangesServer, probeID int64) (checksSet, error) {
	checks, err := s.db.ListChecksForProbe(stream.Context(), probeID)
	if err != nil {
		return nil, err
	}

	m := make(checksSet)

	if len(checks) == 0 && len(currentState.Checks) == 0 {
		return m, nil
	}

	knownChecks := make(map[int64]float64, len(currentState.Checks))
	for _, e := range currentState.Checks {
		knownChecks[e.Id] = e.LastModified
	}

	var changes []sm.CheckChange
	for _, check := range checks {
		if !check.Enabled {
			continue
		}

		modTime, exists := knownChecks[check.Id]
		m[check.Id] = struct{}{}

		if exists && modTime == check.Modified {
			// Skip check already known by probe
			continue
		}

		op := sm.CheckOperation_CHECK_ADD
		if exists {
			// Update already existing check
			op = sm.CheckOperation_CHECK_UPDATE
		}
		changes = append(changes, sm.CheckChange{
			Operation: op,
			Check:     check,
		})
	}

	// Delete missing checks previously known by the probe
	for cid := range knownChecks {
		if _, found := m[cid]; !found {
			changes = append(changes, sm.CheckChange{
				Operation: sm.CheckOperation_CHECK_DELETE,
				Check:     sm.Check{Id: cid},
			})
		}
	}

	s.logger.Info().Int64("probeId", probeID).Interface("changes", changes).Msg("initial changes")

	// If existing checks were provided, notify the probe that we're sending a diff
	// against the existing checks instead of the full batch.
	deltaFlag := len(currentState.Checks) > 0
	if err := stream.Send(&sm.Changes{Checks: changes, IsDeltaFirstBatch: deltaFlag}); err != nil {
		return nil, fmt.Errorf("sending check changes to probe %d: %w", probeID, err)
	}

	return m, nil
}
