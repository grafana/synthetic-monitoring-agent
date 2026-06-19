package cluster

import (
	"testing"

	"github.com/grafana/ckit/shard"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
)

// goldenKeys pins the exact ring key for a set of fixed check IDs. These values
// are part of the wire contract between agents in a ring: a diff here means the
// key encoding changed and agents on different versions would compute different
// owners. Do not update these to match new output; updating them is a breaking
// change that requires a coordinated rollout.
var goldenKeys = map[model.GlobalID]shard.Key{
	0:       3803688792395291579,
	1:       11466160773928732634,
	42:      11728382928442088415,
	1000001: 10717707730743423573,
}

func TestKeyOfGolden(t *testing.T) {
	for id, want := range goldenKeys {
		require.Equalf(t, want, keyOf(id), "key encoding changed for id %d", id)
	}
}

func TestKeyOfDeterministic(t *testing.T) {
	for _, id := range []model.GlobalID{0, 1, -1, 42, 1000001} {
		require.Equal(t, keyOf(id), keyOf(id), "keyOf must be deterministic for id %d", id)
	}
}

func TestKeyOfDistinct(t *testing.T) {
	seen := make(map[shard.Key]model.GlobalID)
	for _, id := range []model.GlobalID{0, 1, 2, 42, 1000001, 9999999} {
		k := keyOf(id)
		if prev, ok := seen[k]; ok {
			t.Fatalf("key collision: ids %d and %d both hash to %d", prev, id, k)
		}
		seen[k] = id
	}
}
