package db

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	db := New()
	require.NotNil(t, db)
}

func TestTenantOps(t *testing.T) {
	db := New()
	require.NotNil(t, db)

	ctx, cancel := testContext(t)
	t.Cleanup(cancel)

	var tenant synthetic_monitoring.Tenant

	require.NoError(t, db.AddTenant(ctx, &tenant))
	require.NotZero(t, tenant.Id)
	require.NotZero(t, tenant.Created)
	require.NotZero(t, tenant.Modified)

	tenants, err := db.ListTenants(ctx)
	require.NoError(t, err)
	require.Len(t, tenants, 1)
	require.Equal(t, tenant, tenants[0])

	newTenant, err := db.UpdateTenant(ctx, &tenant)
	require.NoError(t, err)
	require.Greater(t, newTenant.Modified, newTenant.Created)

	tenants, err = db.ListTenants(ctx)
	require.NoError(t, err)
	require.Len(t, tenants, 1)
	require.Equal(t, *newTenant, tenants[0])

	require.NoError(t, db.DeleteTenant(ctx, tenant.Id))

	tenants, err = db.ListTenants(ctx)
	require.NoError(t, err)
	require.Len(t, tenants, 0)
}

func TestProbeOps(t *testing.T) {
	db := New()
	require.NotNil(t, db)

	ctx, cancel := testContext(t)
	t.Cleanup(cancel)

	var tenant synthetic_monitoring.Tenant

	require.NoError(t, db.AddTenant(ctx, &tenant))

	probe := synthetic_monitoring.Probe{
		Name:     "test",
		TenantId: tenant.Id,
	}
	token := []byte("123")

	require.NoError(t, db.AddProbe(ctx, &probe, token))
	require.NotZero(t, probe.Id)
	require.NotZero(t, probe.Created)
	require.NotZero(t, probe.Modified)

	aProbe, err := db.FindProbeByID(ctx, probe.Id)
	require.NoError(t, err)
	require.NotNil(t, aProbe)
	require.Equal(t, probe, *aProbe)

	aId, err := db.FindProbeIDByToken(ctx, token)
	require.NoError(t, err)
	require.Equal(t, probe.Id, aId)

	probes, err := db.ListProbes(ctx)
	require.NoError(t, err)
	require.Len(t, probes, 1)
	require.Equal(t, probe, probes[0])

	probe.Public = true
	newProbe, err := db.UpdateProbe(ctx, &probe)
	require.NoError(t, err)
	require.Equal(t, probe.Created, newProbe.Created)
	require.Greater(t, newProbe.Modified, newProbe.Created)
	require.Greater(t, newProbe.Modified, probe.Modified)
	probe.Modified = newProbe.Modified // Modified should be the only field that is different
	require.Equal(t, probe, *newProbe)

	probes, err = db.ListProbes(ctx)
	require.NoError(t, err)
	require.Len(t, probes, 1)
	require.Equal(t, probe, probes[0])

	require.NoError(t, db.DeleteProbe(ctx, probe.Id))

	probes, err = db.ListProbes(ctx)
	require.NoError(t, err)
	require.Len(t, probes, 0)
}

func TestCheckOps(t *testing.T) {
	db := New()
	require.NotNil(t, db)

	ctx, cancel := testContext(t)
	t.Cleanup(cancel)

	var tenant synthetic_monitoring.Tenant

	require.NoError(t, db.AddTenant(ctx, &tenant))

	probe := synthetic_monitoring.Probe{
		Name:     "test",
		TenantId: tenant.Id,
	}
	token := []byte("123")

	require.NoError(t, db.AddProbe(ctx, &probe, token))

	check := synthetic_monitoring.Check{
		TenantId:  tenant.Id,
		Probes:    []int64{probe.Id},
		Target:    "127.0.0.1",
		Job:       "test",
		Frequency: 2000,
		Timeout:   2000,
		Settings: synthetic_monitoring.CheckSettings{
			Ping: &synthetic_monitoring.PingSettings{},
		},
	}

	require.NoError(t, db.AddCheck(ctx, &check))
	require.NotZero(t, check.Id)
	require.NotZero(t, check.Created)
	require.NotZero(t, check.Modified)

	aCheck, err := db.GetCheck(ctx, check.Id)
	require.NoError(t, err)
	require.NotNil(t, aCheck)
	require.Equal(t, check, *aCheck)

	checks, err := db.ListChecks(ctx)
	require.NoError(t, err)
	require.Len(t, checks, 1)
	require.Equal(t, check, checks[0])

	check.Frequency = 4000
	oldCheck, err := db.UpdateCheck(ctx, &check)
	require.NoError(t, err)
	require.Equal(t, check.Created, oldCheck.Created)
	require.Equal(t, oldCheck.Modified, oldCheck.Created)
	require.Greater(t, check.Modified, oldCheck.Modified)
	oldCheck.Frequency = check.Frequency
	oldCheck.Modified = check.Modified
	require.Equal(t, check, *oldCheck)

	checks, err = db.ListChecks(ctx)
	require.NoError(t, err)
	require.Len(t, checks, 1)
	require.Equal(t, check, checks[0])

	require.NoError(t, db.DeleteCheck(ctx, check.Id))

	checks, err = db.ListChecks(ctx)
	require.NoError(t, err)
	require.Len(t, checks, 0)
}

func testContext(t *testing.T) (context.Context, func()) {
	if deadline, ok := t.Deadline(); ok {
		return context.WithDeadline(context.Background(), deadline)
	}

	return context.WithTimeout(context.Background(), 10*time.Second)
}
