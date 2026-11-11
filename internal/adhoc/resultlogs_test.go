package adhoc

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// entry returns a log entry whose encoded size is predictable.
func entry(n, padding int) map[string]string {
	return map[string]string{"n": fmt.Sprintf("%04d", n), "pad": strings.Repeat("x", padding)}
}

func totalSize(lines [][]byte) int {
	var size int

	for _, l := range lines {
		size += len(l)
	}

	return size
}

func TestEncodeLogs(t *testing.T) {
	t.Run("no entries", func(t *testing.T) {
		lines, truncated := encodeLogs(nil, 1024)

		require.Empty(t, lines)
		require.False(t, truncated)
	})

	t.Run("everything fits", func(t *testing.T) {
		entries := []map[string]string{entry(1, 10), entry(2, 10), entry(3, 10)}

		lines, truncated := encodeLogs(entries, 1024)

		require.False(t, truncated)
		require.Len(t, lines, 3)

		// Same encoding as the Loki entry, in the original order.
		for i, e := range entries {
			want, err := json.Marshal(e)
			require.NoError(t, err)
			require.Equal(t, string(want), string(lines[i]))
		}
	})

	t.Run("the newest lines are kept, in order", func(t *testing.T) {
		var entries []map[string]string
		for i := range 100 {
			entries = append(entries, entry(i, 100))
		}

		// Each line is a bit over 100 bytes: room for about ten.
		lines, truncated := encodeLogs(entries, 1200)

		require.True(t, truncated)
		require.NotEmpty(t, lines)
		require.Less(t, len(lines), 100)
		require.LessOrEqual(t, totalSize(lines), 1200)

		last := lines[len(lines)-1]
		require.Contains(t, string(last), `"n":"0099"`, "the newest line is always kept")

		// What is kept is a contiguous run of the newest entries, in order.
		first := 100 - len(lines)
		for i, l := range lines {
			require.Contains(t, string(l), fmt.Sprintf(`"n":"%04d"`, first+i))
		}
	})

	t.Run("a line over the limit is replaced and newer lines are kept", func(t *testing.T) {
		entries := []map[string]string{entry(1, 10), entry(2, 5000), entry(3, 10)}

		lines, truncated := encodeLogs(entries, 1000)

		require.True(t, truncated)
		require.LessOrEqual(t, totalSize(lines), 1000)
		require.Contains(t, string(lines[len(lines)-1]), `"n":"0003"`)

		for _, l := range lines {
			require.True(t, json.Valid(l), "every line must be valid JSON: %s", l)
			require.NotContains(t, string(l), strings.Repeat("x", 5000))
		}
	})

	t.Run("when the newest line is over the limit", func(t *testing.T) {
		entries := []map[string]string{entry(1, 10), entry(2, 5000)}

		lines, truncated := encodeLogs(entries, 1000)

		require.True(t, truncated)
		require.LessOrEqual(t, totalSize(lines), 1000)
		require.NotEmpty(t, lines, "there must be something to show")
		require.True(t, json.Valid(lines[len(lines)-1]))
	})
}
