package scraper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

func labels(pairs ...string) []sm.Label {
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
		labels("env", "dev"),
		labels("env", "prod"),
		sm.LabelMode_LABEL_MODE_PREFIXED,
	)
	require.Len(t, result, 1)
	assert.Equal(t, "label_env", result[0].name)
	assert.Equal(t, "prod", result[0].value)
}

// TestBuildUserLabels_Prefixed_TwoDistinctLabels: probe has region=eu, check has env=prod.
// Both appear prefixed, probe label first.
func TestBuildUserLabels_Prefixed_TwoDistinctLabels(t *testing.T) {
	result := buildUserLabels(
		labels("region", "eu"),
		labels("env", "prod"),
		sm.LabelMode_LABEL_MODE_PREFIXED,
	)
	require.Len(t, result, 2)
	lp1 := findLabel(t, result, "label_region")
	assert.Equal(t, "eu", lp1.value)
	lp2 := findLabel(t, result, "label_env")
	assert.Equal(t, "prod", lp2.value)
}

// TestBuildUserLabels_Prefixed_ReservedNotDropped: in PREFIXED mode, reserved names are NOT
// dropped — enforcement is at the API layer, not here.
func TestBuildUserLabels_Prefixed_ReservedNotDropped(t *testing.T) {
	result := buildUserLabels(
		nil,
		labels("probe", "myprobe"),
		sm.LabelMode_LABEL_MODE_PREFIXED,
	)
	require.Len(t, result, 1)
	assert.Equal(t, "label_probe", result[0].name)
	assert.Equal(t, "myprobe", result[0].value)
}

// TestBuildUserLabels_DualWrite_Basic: DUAL_WRITE emits all prefixed forms first, then all
// un-prefixed forms: [label_env, env]. The prefixed form comes first so that in the event
// of log stream label overflow, the form existing policies depend on is preserved.
func TestBuildUserLabels_DualWrite_Basic(t *testing.T) {
	result := buildUserLabels(
		nil,
		labels("env", "prod"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	require.Len(t, result, 2)
	assert.Equal(t, "label_env", result[0].name, "prefixed form must come first")
	assert.Equal(t, "prod", result[0].value)
	assert.Equal(t, "env", result[1].name, "un-prefixed form must come second")
	assert.Equal(t, "prod", result[1].value)
}

// TestBuildUserLabels_DualWrite_TwoLabels: two labels produce [label_a, label_b, a, b] —
// all prefixed first, then all un-prefixed.
func TestBuildUserLabels_DualWrite_TwoLabels(t *testing.T) {
	result := buildUserLabels(
		labels("team", "sre"),
		labels("env", "prod"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	require.Len(t, result, 4)
	// First half: all prefixed
	assert.Equal(t, "label_team", result[0].name)
	assert.Equal(t, "label_env", result[1].name)
	// Second half: all un-prefixed
	assert.Equal(t, "team", result[2].name)
	assert.Equal(t, "env", result[3].name)
}

// TestBuildUserLabels_DualWrite_CheckOverridesProbe: probe env=dev, check env=prod.
// Both forms should carry the check's value.
func TestBuildUserLabels_DualWrite_CheckOverridesProbe(t *testing.T) {
	result := buildUserLabels(
		labels("env", "dev"),
		labels("env", "prod"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	require.Len(t, result, 2)
	assert.Equal(t, "label_env", result[0].name)
	assert.Equal(t, "prod", result[0].value, "prefixed form should have check value")
	assert.Equal(t, "env", result[1].name)
	assert.Equal(t, "prod", result[1].value, "un-prefixed form should have check value")
}

// TestBuildUserLabels_DualWrite_ReservedNamePassthrough: reserved names are NOT dropped —
// user-defined value wins over the system-emitted value for post-migration additions.
func TestBuildUserLabels_DualWrite_ReservedNamePassthrough(t *testing.T) {
	result := buildUserLabels(
		nil,
		labels("probe", "myprobe", "env", "prod"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	// [label_probe, label_env, probe, env]
	require.Len(t, result, 4)
	assert.Equal(t, "label_probe", result[0].name)
	assert.Equal(t, "label_env", result[1].name)
	assert.Equal(t, "probe", result[2].name)
	assert.Equal(t, "env", result[3].name)
}

// TestBuildUserLabels_Unprefixed_Basic: single label env=prod produces only env=prod.
func TestBuildUserLabels_Unprefixed_Basic(t *testing.T) {
	result := buildUserLabels(
		nil,
		labels("env", "prod"),
		sm.LabelMode_LABEL_MODE_UNPREFIXED,
	)
	require.Len(t, result, 1)
	assert.Equal(t, "env", result[0].name)
	assert.Equal(t, "prod", result[0].value)
}

// TestBuildUserLabels_Unprefixed_ReservedNamePassthrough: reserved names are NOT dropped.
func TestBuildUserLabels_Unprefixed_ReservedNamePassthrough(t *testing.T) {
	result := buildUserLabels(
		nil,
		labels("probe", "myprobe", "env", "prod"),
		sm.LabelMode_LABEL_MODE_UNPREFIXED,
	)
	require.Len(t, result, 2)
	findLabel(t, result, "probe")
	findLabel(t, result, "env")
}
