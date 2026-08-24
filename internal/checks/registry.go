package checks

import (
	"sync"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/scraper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// scraperRegistry owns the set of currently-running scrapers and two
// derived views of it: a per-tenant index of the checks each tenant has
// running, and the most recently observed label mode per tenant. All three
// are guarded by mu and are only ever mutated together, so the index can
// never drift from the scraper set. The registry is pure state: it never
// stops scrapers, logs, or records metrics — callers do that with the
// values it returns.
type scraperRegistry struct {
	mu       sync.Mutex
	scrapers map[model.GlobalID]*scraper.Scraper
	// byTenant has a key for a tenant if and only if at least one scraper
	// for that tenant is present in scrapers; the inner set holds those
	// checks' global IDs.
	byTenant map[model.GlobalID]map[model.GlobalID]struct{}
	// labelModes records the most recently observed label mode per tenant.
	// Entries are recorded for any tenant seen in a change batch, running
	// scrapers or not, and are never removed.
	labelModes map[model.GlobalID]sm.LabelMode
}

func newScraperRegistry() *scraperRegistry {
	return &scraperRegistry{
		scrapers:   make(map[model.GlobalID]*scraper.Scraper),
		byTenant:   make(map[model.GlobalID]map[model.GlobalID]struct{}),
		labelModes: make(map[model.GlobalID]sm.LabelMode),
	}
}

func (r *scraperRegistry) get(id model.GlobalID) (*scraper.Scraper, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, found := r.scrapers[id]

	return s, found
}

func (r *scraperRegistry) add(check model.Check, s *scraper.Scraper) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cid := check.GlobalID()
	tid := check.GlobalTenantID()

	r.scrapers[cid] = s

	checks, found := r.byTenant[tid]
	if !found {
		checks = make(map[model.GlobalID]struct{})
		r.byTenant[tid] = checks
	}

	checks[cid] = struct{}{}
}

func (r *scraperRegistry) remove(id model.GlobalID) (*scraper.Scraper, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.removeLocked(id)
}

// removeLocked requires r.mu to be held by the caller.
func (r *scraperRegistry) removeLocked(id model.GlobalID) (*scraper.Scraper, bool) {
	s, found := r.scrapers[id]
	if !found {
		return nil, false
	}

	delete(r.scrapers, id)

	// The tenant is derived from the removed scraper's own check rather
	// than trusted from the caller, so the index entry removed here is
	// always the one add created.
	check := s.Check()
	tid := check.GlobalTenantID()

	checks := r.byTenant[tid]
	delete(checks, id)

	if len(checks) == 0 {
		delete(r.byTenant, tid)
	}

	return s, true
}

// removeAbsent removes every scraper whose check ID is not in keep and
// returns the removed scrapers so the caller can stop them.
func (r *scraperRegistry) removeAbsent(keep map[model.GlobalID]struct{}) []*scraper.Scraper {
	r.mu.Lock()
	defer r.mu.Unlock()

	var removed []*scraper.Scraper

	for id := range r.scrapers {
		if _, found := keep[id]; found {
			continue
		}

		if s, ok := r.removeLocked(id); ok {
			removed = append(removed, s)
		}
	}

	return removed
}

// checksForTenant returns a fresh snapshot of the checks tenant has running
// scrapers for; callers may mutate the registry while iterating it.
func (r *scraperRegistry) checksForTenant(tenant model.GlobalID) []model.Check {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := r.byTenant[tenant]

	checks := make([]model.Check, 0, len(ids))
	for id := range ids {
		checks = append(checks, r.scrapers[id].Check())
	}

	return checks
}

func (r *scraperRegistry) entityRefs() []sm.EntityRef {
	r.mu.Lock()
	defer r.mu.Unlock()

	refs := make([]sm.EntityRef, 0, len(r.scrapers))
	for id, s := range r.scrapers {
		refs = append(refs, sm.EntityRef{
			Id:           int64(id),
			LastModified: s.LastModified(),
		})
	}

	return refs
}

// setLabelMode records mode as tenant's current label mode and reports
// whether that differs from the previous record. A tenant never seen
// before counts as changed: LabelMode's zero value is PREFIXED, so an
// absent entry cannot be told apart from "seen at PREFIXED" without the
// two-value read, and treating unseen as unchanged would reopen the
// staleness bug the restart-on-mode-change feature exists to fix.
func (r *scraperRegistry) setLabelMode(tenant model.GlobalID, mode sm.LabelMode) (changed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	previous, seen := r.labelModes[tenant]
	r.labelModes[tenant] = mode

	return !seen || previous != mode
}
