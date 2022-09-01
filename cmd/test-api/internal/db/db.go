package db

import (
	"context"
	"errors"
	"sync"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type Db struct {
	mutex         sync.Mutex
	tenants       map[int64]sm.Tenant
	probes        map[int64]sm.Probe
	probeTokens   map[string]int64
	checks        map[int64]sm.Check
	checksByProbe map[int64][]int64
}

func New() *Db {
	return &Db{
		tenants:       make(map[int64]sm.Tenant),
		probes:        make(map[int64]sm.Probe),
		probeTokens:   make(map[string]int64),
		checks:        make(map[int64]sm.Check),
		checksByProbe: make(map[int64][]int64),
	}
}

func (db *Db) FindProbeIDByToken(ctx context.Context, token []byte) (int64, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if id, found := db.probeTokens[string(token)]; found {
		return id, nil
	}

	return -1, errors.New("token not found")
}

func (db *Db) FindProbeByID(ctx context.Context, id int64) (*sm.Probe, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if probe, found := db.probes[id]; found {
		return &probe, nil
	}

	return nil, errors.New("probe not found")
}

func (db *Db) ListChecksForProbe(ctx context.Context, id int64) ([]sm.Check, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	checks := make([]sm.Check, 0, len(db.checksByProbe[id]))

	for _, checkId := range db.checksByProbe[id] {
		checks = append(checks, db.checks[checkId])
	}

	return checks, nil
}

func (db *Db) AddTenant(ctx context.Context, tenant *sm.Tenant) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	for _, t := range db.tenants {
		if t.StackId == tenant.StackId {
			return errors.New("tenant already exists")
		}
	}

	now := nsNow()
	newTenant := sm.Tenant{
		Id:       int64(len(db.tenants) + 1),
		OrgId:    tenant.OrgId,
		StackId:  tenant.StackId,
		Status:   sm.TenantStatus_ACTIVE,
		Reason:   "",
		Created:  now,
		Modified: now,
	}
	if tenant.MetricsRemote != nil {
		newTenant.MetricsRemote = &sm.RemoteInfo{
			Name:     tenant.MetricsRemote.Name,
			Url:      tenant.MetricsRemote.Url,
			Username: tenant.MetricsRemote.Username,
			Password: tenant.MetricsRemote.Password,
		}
	}
	if tenant.EventsRemote != nil {
		newTenant.EventsRemote = &sm.RemoteInfo{
			Name:     tenant.EventsRemote.Name,
			Url:      tenant.EventsRemote.Url,
			Username: tenant.EventsRemote.Username,
			Password: tenant.EventsRemote.Password,
		}
	}
	db.tenants[newTenant.Id] = newTenant

	*tenant = newTenant

	return nil
}

func (db *Db) GetTenant(ctx context.Context, id int64) (*sm.Tenant, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	tenant, found := db.tenants[id]
	if !found {
		return nil, errors.New("tenant not found")
	}

	return &tenant, nil
}

func (db *Db) ListTenants(ctx context.Context) ([]sm.Tenant, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	tenants := make([]sm.Tenant, 0, len(db.tenants))
	for _, tenant := range db.tenants {
		tenants = append(tenants, tenant)
	}

	return tenants, nil
}

func (db *Db) UpdateTenant(ctx context.Context, tenant *sm.Tenant) (*sm.Tenant, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if _, found := db.tenants[tenant.Id]; !found {
		return nil, errors.New("tenant not found")
	}

	tenant.Modified = nsNow()
	db.tenants[tenant.Id] = *tenant

	return tenant, nil
}

func (db *Db) DeleteTenant(ctx context.Context, id int64) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if _, found := db.tenants[id]; !found {
		return errors.New("tenant not found")
	}

	for _, probe := range db.probes {
		if probe.TenantId == id {
			return errors.New("tenant is still referenced by probe")
		}
	}

	for _, check := range db.checks {
		if check.TenantId == id {
			return errors.New("tenant is still referenced by check")
		}
	}

	delete(db.tenants, id)

	return nil
}

func (db *Db) AddProbe(ctx context.Context, probe *sm.Probe, token []byte) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if err := probe.Validate(); err != nil {
		return err
	}

	if _, found := db.tenants[probe.TenantId]; !found {
		return errors.New("invalid tenant")
	}

	for _, p := range db.probes {
		// if the new probe is public, it cannot use an existing name in any of the probes
		// if the existing probe is public, the new probe cannot use its name
		// if they are private and both are in the same tenant, they cannot have the same name
		if (probe.Public || p.TenantId == probe.TenantId || p.Public) && p.Name == probe.Name {
			return errors.New("probe already exists")
		}
	}

	if _, found := db.probeTokens[string(token)]; found {
		return errors.New("invalid probe token")
	}

	probe.Id = int64(len(db.probes) + 1)
	probe.Created = nsNow()
	probe.Modified = probe.Created

	db.probes[probe.Id] = *probe
	db.probeTokens[string(token)] = probe.Id

	return nil
}

func (db *Db) GetProbe(ctx context.Context, id int64) (*sm.Probe, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if probe, found := db.probes[id]; found {
		return &probe, nil
	}

	return nil, errors.New("probe not found")
}

func (db *Db) ListProbes(ctx context.Context) ([]sm.Probe, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	probes := make([]sm.Probe, 0, len(db.probes))

	for _, probe := range db.probes {
		probes = append(probes, probe)
	}

	return probes, nil
}

func (db *Db) UpdateProbe(ctx context.Context, probe *sm.Probe) (*sm.Probe, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	oldProbe, found := db.probes[probe.Id]
	if !found {
		return nil, errors.New("probe not found")
	}

	if oldProbe.Name != probe.Name {
		return nil, errors.New("cannot rename probes")
	}

	if oldProbe.TenantId != probe.TenantId {
		return nil, errors.New("cannot move probes between tenants")
	}

	if err := probe.Validate(); err != nil {
		return nil, err
	}

	for _, p := range db.probes {
		if p.Id == probe.Id {
			continue
		}

		// If the upated probe is public (or becoming public), it
		// cannot share the name with any other probe.
		if probe.Public && p.Name == probe.Name {
			return nil, errors.New("invalid probe name")
		}
	}

	updatedProbe := *probe
	updatedProbe.Modified = nsNow()

	db.probes[probe.Id] = updatedProbe

	return &updatedProbe, nil
}

func (db *Db) DeleteProbe(ctx context.Context, id int64) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if _, found := db.probes[id]; !found {
		return errors.New("probe not found")
	}

	for _, check := range db.checks {
		for _, pId := range check.Probes {
			if pId == id {
				return errors.New("probe is still referenced by check")
			}
		}
	}

	for token, probeId := range db.probeTokens {
		if probeId == id {
			delete(db.probeTokens, token)
		}
	}

	delete(db.probes, id)

	return nil
}

func (db *Db) AddCheck(ctx context.Context, check *sm.Check) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if err := check.Validate(); err != nil {
		return err
	}

	if _, found := db.tenants[check.TenantId]; !found {
		return errors.New("invalid tenant")
	}

	for _, c := range db.checks {
		if c.Target == check.Target && c.Job == check.Job {
			return errors.New("check already exists")
		}
	}

	for _, pId := range check.Probes {
		probe, found := db.probes[pId]
		if !found {
			return errors.New("probe not found")
		}

		if probe.TenantId != check.TenantId && !probe.Public {
			return errors.New("invalid probe")
		}
	}

	check.Id = int64(len(db.checks) + 1)
	check.Created = nsNow()
	check.Modified = check.Created

	db.checks[check.Id] = *check

	// now that we now everything is valid, add checks to probes
	for _, pId := range check.Probes {
		db.checksByProbe[pId] = append(db.checksByProbe[pId], check.Id)
	}

	return nil
}

func (db *Db) GetCheck(ctx context.Context, id int64) (*sm.Check, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if check, found := db.checks[id]; found {
		return &check, nil
	}

	return nil, errors.New("check not found")
}

func (db *Db) ListChecks(ctx context.Context) ([]sm.Check, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	checks := make([]sm.Check, 0, len(db.checks))

	for _, check := range db.checks {
		checks = append(checks, check)
	}

	return checks, nil
}

func (db *Db) UpdateCheck(ctx context.Context, check *sm.Check) (*sm.Check, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	oldCheck, found := db.checks[check.Id]
	if !found {
		return nil, errors.New("check not found")
	}

	if oldCheck.Job != check.Job {
		return nil, errors.New("cannot change job")
	}

	if oldCheck.Target != check.Target {
		return nil, errors.New("cannot change target")
	}

	if oldCheck.TenantId != check.TenantId {
		return nil, errors.New("cannot move checks between tenants")
	}

	if err := check.Validate(); err != nil {
		return nil, err
	}

	check.Modified = nsNow()

	db.checks[check.Id] = *check

	return &oldCheck, nil
}

func (db *Db) DeleteCheck(ctx context.Context, id int64) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if _, found := db.checks[id]; !found {
		return errors.New("check not found")
	}

	delete(db.checks, id)

	return nil
}

func nsNow() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}
