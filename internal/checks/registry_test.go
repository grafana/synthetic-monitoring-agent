package checks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/grafana/synthetic-monitoring-agent/internal/scraper"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// requireRegistryConsistent rebuilds the tenant index from the scraper map
// from scratch and requires it to match byTenant exactly, catching any
// drift a code path could introduce between the two. It also requires that
// no empty inner set survives, pinning the "byTenant key present iff the
// tenant has at least one running scraper" invariant.
func requireRegistryConsistent(t *testing.T, r *scraperRegistry) {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	want := make(map[model.GlobalID]map[model.GlobalID]struct{})

	for id, s := range r.scrapers {
		check := s.Check()
		tid := check.GlobalTenantID()

		if want[tid] == nil {
			want[tid] = make(map[model.GlobalID]struct{})
		}

		want[tid][id] = struct{}{}
	}

	require.Equal(t, want, r.byTenant, "tenant index has drifted from the scraper map")

	for tid, checks := range r.byTenant {
		require.NotEmpty(t, checks, "tenant %d has an empty index entry", tid)
	}
}

// registryLen reaches into the registry under its lock, the same way tests
// read labelModes; the registry deliberately exposes no size accessor
// because production code has no use for one.
func registryLen(r *scraperRegistry) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.scrapers)
}

func newRegistryTestCheck(t *testing.T, id, tenantID int64) model.Check {
	t.Helper()

	var check model.Check

	err := check.FromSM(sm.Check{
		Id:        id,
		TenantId:  tenantID,
		Frequency: 1000,
		Timeout:   1000,
		Target:    "127.0.0.1",
		Job:       "test-job",
		Probes:    []int64{1},
		Settings:  sm.CheckSettings{Ping: &sm.PingSettings{}},
	})
	require.NoError(t, err)

	return check
}

// newRegistryTestScraper builds a real *scraper.Scraper for check without
// ever running it, so the zero-value collaborators are never exercised and
// no goroutine needs stopping.
func newRegistryTestScraper(t *testing.T, check model.Check) *scraper.Scraper {
	t.Helper()

	s, err := testScraperFactory(
		context.Background(),
		check,
		channelPublisher(make(chan pusher.Payload, 1)),
		sm.Probe{Id: 100, Name: "test-probe"},
		nil,
		testhelper.Logger(t),
		nil,
		nil, nil, nil, nil, nil, nil,
	)
	require.NoError(t, err)

	return s
}

func TestScraperRegistry(t *testing.T) {
	r := newScraperRegistry()
	requireRegistryConsistent(t, r)
	require.Zero(t, registryLen(r))

	checkA := newRegistryTestCheck(t, 1, 42)
	checkB := newRegistryTestCheck(t, 2, 42)
	checkC := newRegistryTestCheck(t, 3, 99)

	tenant42 := checkA.GlobalTenantID()
	tenant99 := checkC.GlobalTenantID()

	scraperA := newRegistryTestScraper(t, checkA)
	scraperB := newRegistryTestScraper(t, checkB)
	scraperC := newRegistryTestScraper(t, checkC)

	r.add(checkA, scraperA)
	r.add(checkB, scraperB)
	r.add(checkC, scraperC)
	requireRegistryConsistent(t, r)
	require.Equal(t, 3, registryLen(r))

	got, found := r.get(checkA.GlobalID())
	require.True(t, found)
	require.Same(t, scraperA, got)

	_, found = r.get(model.GlobalID(12345))
	require.False(t, found)

	require.ElementsMatch(t, []model.Check{checkA, checkB}, r.checksForTenant(tenant42))
	require.ElementsMatch(t, []model.Check{checkC}, r.checksForTenant(tenant99))

	refs := r.entityRefs()
	require.ElementsMatch(t,
		[]int64{int64(checkA.GlobalID()), int64(checkB.GlobalID()), int64(checkC.GlobalID())},
		[]int64{refs[0].Id, refs[1].Id, refs[2].Id})

	// removing one of tenant 42's checks leaves the other indexed
	removed, found := r.remove(checkA.GlobalID())
	require.True(t, found)
	require.Same(t, scraperA, removed)
	requireRegistryConsistent(t, r)
	require.Equal(t, 2, registryLen(r))
	require.ElementsMatch(t, []model.Check{checkB}, r.checksForTenant(tenant42))

	// removing tenant 42's last check deletes its index key entirely
	removed, found = r.remove(checkB.GlobalID())
	require.True(t, found)
	require.Same(t, scraperB, removed)
	requireRegistryConsistent(t, r)
	require.Empty(t, r.checksForTenant(tenant42))

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		_, exists := r.byTenant[tenant42]
		require.False(t, exists, "a tenant with no running scrapers must not keep an index key")
	}()

	// removing an absent ID reports absence and mutates nothing
	removed, found = r.remove(checkA.GlobalID())
	require.False(t, found)
	require.Nil(t, removed)
	require.Equal(t, 1, registryLen(r))
	requireRegistryConsistent(t, r)

	// re-populate tenant 42 so the sweep below has something to remove
	scraperA2 := newRegistryTestScraper(t, checkA)
	scraperB2 := newRegistryTestScraper(t, checkB)

	r.add(checkA, scraperA2)
	r.add(checkB, scraperB2)
	requireRegistryConsistent(t, r)
	require.Equal(t, 3, registryLen(r))

	swept := r.removeAbsent(map[model.GlobalID]struct{}{checkC.GlobalID(): {}})
	require.ElementsMatch(t, []*scraper.Scraper{scraperA2, scraperB2}, swept)
	requireRegistryConsistent(t, r)
	require.Equal(t, 1, registryLen(r))
	require.Empty(t, r.checksForTenant(tenant42))
	require.ElementsMatch(t, []model.Check{checkC}, r.checksForTenant(tenant99))

	// setLabelMode truth table. The first sighting uses PREFIXED, the
	// enum's zero value, to pin that "never seen" is distinguished from
	// "seen at PREFIXED".
	require.True(t, r.setLabelMode(tenant42, sm.LabelMode_LABEL_MODE_PREFIXED),
		"first sighting of a tenant must count as changed even at the zero-value mode")
	require.False(t, r.setLabelMode(tenant42, sm.LabelMode_LABEL_MODE_PREFIXED),
		"repeating the recorded mode must not count as changed")
	require.True(t, r.setLabelMode(tenant42, sm.LabelMode_LABEL_MODE_DUAL_WRITE),
		"a different mode must count as changed")

	// record-always: a tenant with zero running scrapers still records
	noScrapersTenant := model.GlobalID(7)
	require.True(t, r.setLabelMode(noScrapersTenant, sm.LabelMode_LABEL_MODE_UNPREFIXED))
	require.False(t, r.setLabelMode(noScrapersTenant, sm.LabelMode_LABEL_MODE_UNPREFIXED))
	requireRegistryConsistent(t, r)
}
