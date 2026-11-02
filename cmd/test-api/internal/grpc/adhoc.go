package grpc

import (
	"context"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
)

// AdHocChecksServer is a minimal implementation of the ad-hoc checks service.
// It accepts registrations and never sends any ad-hoc checks. Without it the
// agent treats the API as incompatible and exits.
type AdHocChecksServer struct {
	logger zerolog.Logger
	db     ChecksDb
}

type AdHocChecksServerOpts struct {
	Logger zerolog.Logger
	Db     ChecksDb
}

func NewAdHocChecksServer(opts AdHocChecksServerOpts) (*AdHocChecksServer, error) {
	return &AdHocChecksServer{
		logger: opts.Logger,
		db:     opts.Db,
	}, nil
}

func (s *AdHocChecksServer) RegisterProbe(ctx context.Context, _ *sm.ProbeInfo) (*sm.RegisterProbeResult, error) {
	probeID, found := probeIdFromContext(ctx)
	if !found {
		return &sm.RegisterProbeResult{
			Status: sm.Status{
				Code:    sm.StatusCode_NOT_AUTHORIZED,
				Message: "cannot get probe ID",
			},
		}, nil
	}

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

func (s *AdHocChecksServer) GetAdHocChecks(_ *sm.Void, stream sm.AdHocChecks_GetAdHocChecksServer) error {
	<-stream.Context().Done()

	return nil
}
