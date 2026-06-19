package cluster

import (
	"encoding/binary"

	"github.com/grafana/ckit/shard"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
)

// keyOf maps a check's GlobalID to a ring key. The encoding MUST stay stable
// across agent versions: every agent in a ring has to derive the same key for
// the same check, or ownership diverges. Changing it is a breaking change.
func keyOf(id model.GlobalID) shard.Key {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(id))
	kb := shard.NewKeyBuilder()
	_, _ = kb.Write(b[:]) // KeyBuilder.Write never returns an error.
	return kb.Key()
}
