package scraper

import (
	"testing"

	"github.com/stretchr/testify/require"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

func makeLabels(pairs ...string) []sm.Label {
	if len(pairs)%2 != 0 {
		panic("labels: pairs must be even")
	}
	out := make([]sm.Label, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, sm.Label{Name: pairs[i], Value: pairs[i+1]})
	}
	return out
}

func findLabel(t *testing.T, lps []labelPair, name string) labelPair {
	t.Helper()
	for _, lp := range lps {
		if lp.name == name {
			return lp
		}
	}
	t.Fatalf("label %q not found in %v", name, lps)
	return labelPair{}
}

func assertNoLabel(t *testing.T, lps []labelPair, name string) {
	t.Helper()
	for _, lp := range lps {
		if lp.name == name {
			t.Errorf("unexpected label %q found in %v", name, lps)
		}
	}
}

// TestBuildUserLabels_Prefixed_CheckOverridesProbe: probe sets env=dev, check sets env=prod.
// Result should be only label_env=prod (check wins, prefixed).
func TestBuildUserLabels_Prefixed_CheckOverridesProbe(t *testing.T) {
	result := buildUserLabels(
		makeLabels("env", "dev"),
		makeLabels("env", "prod"),
		sm.LabelMode_LABEL_MODE_PREFIXED,
	)
	require.Len(t, result, 1)
	require.Equal(t, "label_env", result[0].name)
	require.Equal(t, "prod", result[0].value)
}

// TestBuildUserLabels_Prefixed_TwoDistinctLabels: probe has region=eu, check has env=prod.
// Both appear prefixed, probe label first.
func TestBuildUserLabels_Prefixed_TwoDistinctLabels(t *testing.T) {
	result := buildUserLabels(
		makeLabels("region", "eu"),
		makeLabels("env", "prod"),
		sm.LabelMode_LABEL_MODE_PREFIXED,
	)
	require.Len(t, result, 2)
	lp1 := findLabel(t, result, "label_region")
	require.Equal(t, "eu", lp1.value)
	lp2 := findLabel(t, result, "label_env")
	require.Equal(t, "prod", lp2.value)
}

// TestBuildUserLabels_Prefixed_ReservedNotDropped: in PREFIXED mode, reserved names are NOT
// dropped — enforcement is at the API layer, not here.
func TestBuildUserLabels_Prefixed_ReservedNotDropped(t *testing.T) {
	result := buildUserLabels(
		nil,
		makeLabels("probe", "myprobe"),
		sm.LabelMode_LABEL_MODE_PREFIXED,
	)
	require.Len(t, result, 1)
	require.Equal(t, "label_probe", result[0].name)
	require.Equal(t, "myprobe", result[0].value)
}

// TestBuildUserLabels_DualWrite_Basic: DUAL_WRITE emits all prefixed forms first, then all
// un-prefixed forms: [label_env, env]. The prefixed form comes first so that in the event
// of log stream label overflow, the form existing policies depend on is preserved.
func TestBuildUserLabels_DualWrite_Basic(t *testing.T) {
	result := buildUserLabels(
		nil,
		makeLabels("env", "prod"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	require.Len(t, result, 2)
	require.Equal(t, "label_env", result[0].name, "prefixed form must come first")
	require.Equal(t, "prod", result[0].value)
	require.Equal(t, "env", result[1].name, "un-prefixed form must come second")
	require.Equal(t, "prod", result[1].value)
}

// TestBuildUserLabels_DualWrite_TwoLabels: two labels produce [label_a, label_b, a, b] —
// all prefixed first, then all un-prefixed.
func TestBuildUserLabels_DualWrite_TwoLabels(t *testing.T) {
	result := buildUserLabels(
		makeLabels("team", "sre"),
		makeLabels("env", "prod"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	require.Len(t, result, 4)
	// First half: all prefixed
	require.Equal(t, "label_team", result[0].name)
	require.Equal(t, "label_env", result[1].name)
	// Second half: all un-prefixed
	require.Equal(t, "team", result[2].name)
	require.Equal(t, "env", result[3].name)
}

// TestBuildUserLabels_DualWrite_CheckOverridesProbe: probe env=dev, check env=prod.
// Both forms should carry the check's value.
func TestBuildUserLabels_DualWrite_CheckOverridesProbe(t *testing.T) {
	result := buildUserLabels(
		makeLabels("env", "dev"),
		makeLabels("env", "prod"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	require.Len(t, result, 2)
	require.Equal(t, "label_env", result[0].name)
	require.Equal(t, "prod", result[0].value, "prefixed form should have check value")
	require.Equal(t, "env", result[1].name)
	require.Equal(t, "prod", result[1].value, "un-prefixed form should have check value")
}

// TestBuildUserLabels_DualWrite_ReservedNamePassthrough: reserved names are NOT dropped —
// user-defined value wins over the system-emitted value for post-migration additions.
func TestBuildUserLabels_DualWrite_ReservedNamePassthrough(t *testing.T) {
	result := buildUserLabels(
		nil,
		makeLabels("probe", "myprobe", "env", "prod"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	// [label_probe, label_env, probe, env]
	require.Len(t, result, 4)
	require.Equal(t, "label_probe", result[0].name)
	require.Equal(t, "label_env", result[1].name)
	require.Equal(t, "probe", result[2].name)
	require.Equal(t, "env", result[3].name)
}

// TestBuildUserLabels_Unprefixed_Basic: single label env=prod produces only env=prod.
func TestBuildUserLabels_Unprefixed_Basic(t *testing.T) {
	result := buildUserLabels(
		nil,
		makeLabels("env", "prod"),
		sm.LabelMode_LABEL_MODE_UNPREFIXED,
	)
	require.Len(t, result, 1)
	require.Equal(t, "env", result[0].name)
	require.Equal(t, "prod", result[0].value)
}

// TestBuildUserLabels_Unprefixed_ReservedNamePassthrough: reserved names are NOT dropped.
func TestBuildUserLabels_Unprefixed_ReservedNamePassthrough(t *testing.T) {
	result := buildUserLabels(
		nil,
		makeLabels("probe", "myprobe", "env", "prod"),
		sm.LabelMode_LABEL_MODE_UNPREFIXED,
	)
	require.Len(t, result, 2)
	findLabel(t, result, "probe")
	findLabel(t, result, "env")
}

// ── customMetricLabels tests ─────────────────────────────────────────────

// TestUserLabelsForExecution_Prefixed: PREFIXED mode returns nil — no user labels on execution metrics.
func TestUserLabelsForExecution_Prefixed(t *testing.T) {
	result := customMetricLabels(
		makeLabels("env", "prod"),
		makeLabels("team", "platform"),
		sm.LabelMode_LABEL_MODE_PREFIXED,
	)
	require.Nil(t, result, "PREFIXED mode should return nil — execution metrics carry no user labels")
}

// TestUserLabelsForExecution_DualWrite: DUAL_WRITE returns only un-prefixed form.
func TestUserLabelsForExecution_DualWrite(t *testing.T) {
	result := customMetricLabels(
		nil,
		makeLabels("env", "prod", "team", "platform"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	require.Len(t, result, 2, "should emit one entry per user label (un-prefixed only)")
	findLabel(t, result, "env")
	findLabel(t, result, "team")
	assertNoLabel(t, result, "label_env")
	assertNoLabel(t, result, "label_team")
}

// TestUserLabelsForExecution_Unprefixed: UNPREFIXED returns un-prefixed form.
func TestUserLabelsForExecution_Unprefixed(t *testing.T) {
	result := customMetricLabels(
		nil,
		makeLabels("env", "prod"),
		sm.LabelMode_LABEL_MODE_UNPREFIXED,
	)
	require.Len(t, result, 1)
	require.Equal(t, "env", result[0].name)
	require.Equal(t, "prod", result[0].value)
}

// TestUserLabelsForExecution_ReservedNamePassthrough: reserved names are NOT dropped —
// if a tenant reaches this state post-migration it means we added a new reserved label,
// and the user-defined value should win to avoid breaking existing behaviour.
func TestUserLabelsForExecution_ReservedNamePassthrough(t *testing.T) {
	for _, mode := range []sm.LabelMode{sm.LabelMode_LABEL_MODE_DUAL_WRITE, sm.LabelMode_LABEL_MODE_UNPREFIXED} {
		result := customMetricLabels(
			nil,
			makeLabels("probe", "myprobe", "env", "prod"),
			mode,
		)
		require.Len(t, result, 2)
		findLabel(t, result, "probe")
		findLabel(t, result, "env")
	}
}

// TestUserLabelsForExecution_CheckOverridesProbe: check label wins over probe label for the same name.
func TestUserLabelsForExecution_CheckOverridesProbe(t *testing.T) {
	result := customMetricLabels(
		makeLabels("env", "staging"),
		makeLabels("env", "prod"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	require.Len(t, result, 1)
	require.Equal(t, "prod", result[0].value, "check label should override probe label")
}
