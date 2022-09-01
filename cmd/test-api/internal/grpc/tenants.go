package grpc

import (
	"context"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
)

type TenantsDb interface {
	GetTenant(ctx context.Context, tenantId int64) (*sm.Tenant, error)
}

type TenantsServerOpts struct {
	Logger zerolog.Logger
	Db     TenantsDb
}

type TenantsServer struct {
	logger zerolog.Logger
	db     TenantsDb
}

func NewTenantsServer(opts TenantsServerOpts) (*TenantsServer, error) {
	return &TenantsServer{
		logger: opts.Logger,
		db:     opts.Db,
	}, nil
}

func (s *TenantsServer) GetTenant(ctx context.Context, info *sm.TenantInfo) (*sm.Tenant, error) {
	tenant, err := s.db.GetTenant(ctx, info.Id)
	if err != nil {
		return nil, err
	}

	return tenant, nil
}
