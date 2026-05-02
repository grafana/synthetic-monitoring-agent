package tiermap

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

const sampleYAML = `
browser:
  default: browser-A
  tenants:
    "1234": browser-B
    "5678": browser-B
small:
  default: small
`

func TestFamilyForCheckType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		checkType string
		family    string
		wantErr   bool
	}{
		{"scripted", FamilySmall, false},
		{"multihttp", FamilySmall, false},
		{"browser", FamilyBrowser, false},
		{"http", "", true},
		{"", "", true},
	}

	for _, tc := range cases {
		fam, err := FamilyForCheckType(tc.checkType)
		if tc.wantErr {
			require.Error(t, err, tc.checkType)
			continue
		}
		require.NoError(t, err, tc.checkType)
		require.Equal(t, tc.family, fam)
	}
}

func TestNew_RejectsMissingDefaults(t *testing.T) {
	t.Parallel()

	_, err := New([]byte(`browser: {default: browser-A}`))
	require.Error(t, err)

	_, err = New([]byte(`small: {default: small}`))
	require.Error(t, err)
}

func TestNew_RejectsMalformed(t *testing.T) {
	t.Parallel()

	_, err := New([]byte(`browser: [not, a, mapping]`))
	require.Error(t, err)
}

func TestMapper_Tier(t *testing.T) {
	t.Parallel()

	m, err := New([]byte(sampleYAML))
	require.NoError(t, err)

	t.Run("browser tenant override", func(t *testing.T) {
		tier, fam, err := m.Tier("browser", "1234")
		require.NoError(t, err)
		require.Equal(t, "browser-B", tier)
		require.Equal(t, FamilyBrowser, fam)
	})

	t.Run("browser default for unlisted tenant", func(t *testing.T) {
		tier, fam, err := m.Tier("browser", "9999")
		require.NoError(t, err)
		require.Equal(t, "browser-A", tier)
		require.Equal(t, FamilyBrowser, fam)
	})

	t.Run("scripted lands on small", func(t *testing.T) {
		tier, fam, err := m.Tier("scripted", "1234")
		require.NoError(t, err)
		require.Equal(t, "small", tier)
		require.Equal(t, FamilySmall, fam)
	})

	t.Run("multihttp lands on small", func(t *testing.T) {
		tier, fam, err := m.Tier("multihttp", "1234")
		require.NoError(t, err)
		require.Equal(t, "small", tier)
		require.Equal(t, FamilySmall, fam)
	})

	t.Run("unknown check type errors", func(t *testing.T) {
		_, _, err := m.Tier("ping", "1234")
		require.ErrorIs(t, err, ErrUnknownCheckType)
	})

	t.Run("tenant override does not cross family", func(t *testing.T) {
		// Tenant 1234 has a browser-B entry; that must not leak into the small family.
		tier, fam, err := m.Tier("scripted", "1234")
		require.NoError(t, err)
		require.Equal(t, "small", tier)
		require.Equal(t, FamilySmall, fam)
	})
}

func TestMapper_KnownTiers(t *testing.T) {
	t.Parallel()

	m, err := New([]byte(sampleYAML))
	require.NoError(t, err)

	known := m.KnownTiers()
	require.ElementsMatch(t, []string{"browser-A", "browser-B", "small"}, known)
}

func TestLive_ReloadAtomicSwap(t *testing.T) {
	t.Parallel()

	initial, err := New([]byte(sampleYAML))
	require.NoError(t, err)

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	live := NewLive(initial, metrics, zerolog.Nop())

	tier, _, err := live.Tier("browser", "1234")
	require.NoError(t, err)
	require.Equal(t, "browser-B", tier)

	updated := `
browser:
  default: browser-A
  tenants:
    "1234": browser-A
small:
  default: small
`
	require.NoError(t, live.Reload([]byte(updated)))

	tier, _, err = live.Tier("browser", "1234")
	require.NoError(t, err)
	require.Equal(t, "browser-A", tier)
}

func TestLive_ReloadKeepsLastKnownGoodOnParseError(t *testing.T) {
	t.Parallel()

	initial, err := New([]byte(sampleYAML))
	require.NoError(t, err)

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	live := NewLive(initial, metrics, zerolog.Nop())

	require.Error(t, live.Reload([]byte("not: [valid, yaml")))

	// Live should still resolve the original mapping.
	tier, _, err := live.Tier("browser", "1234")
	require.NoError(t, err)
	require.Equal(t, "browser-B", tier)

	got := testutil.ToFloat64(metrics.MappingErrors.WithLabelValues(CauseParseError))
	require.Equal(t, float64(1), got)
}

func TestLive_WatchPicksUpFileChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "mapping.yaml")
	require.NoError(t, os.WriteFile(path, []byte(sampleYAML), 0o600))

	initial, err := New([]byte(sampleYAML))
	require.NoError(t, err)

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	live := NewLive(initial, metrics, zerolog.Nop())

	ctx := t.Context()

	go live.Watch(ctx, path, 20*time.Millisecond)

	// Bump mtime forward and rewrite with a different mapping.
	updated := `
browser:
  default: browser-B
small:
  default: small
`
	// Sleep briefly so the new file's mtime is strictly after the watcher's recorded baseline.
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, os.WriteFile(path, []byte(updated), 0o600))
	// Force a noticeable mtime delta on filesystems with coarse resolution.
	now := time.Now().Add(time.Second)
	require.NoError(t, os.Chtimes(path, now, now))

	require.Eventually(t, func() bool {
		tier, _, err := live.Tier("browser", "1234")
		return err == nil && tier == "browser-B"
	}, time.Second, 10*time.Millisecond)
}
