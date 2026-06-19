package cluster

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
)

func TestMonoNode(t *testing.T) {
	n := NewMono()

	require.True(t, n.Ready())

	for _, id := range []model.GlobalID{0, 1, 42, 1000001} {
		owner, err := n.IsOwner(id)
		require.NoError(t, err)
		require.True(t, owner, "noop node must own every check (id %d)", id)
	}
}
