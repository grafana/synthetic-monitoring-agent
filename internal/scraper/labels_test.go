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

// TestBuildUserLabels_DualWrite_Basic: single label env=prod produces both env=prod and label_env=prod.
func TestBuildUserLabels_DualWrite_Basic(t *testing.T) {
	result := buildUserLabels(
		nil,
		labels("env", "prod"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	require.Len(t, result, 2)
	unprefixed := findLabel(t, result, "env")
	assert.Equal(t, "prod", unprefixed.value)
	prefixed := findLabel(t, result, "label_env")
	assert.Equal(t, "prod", prefixed.value)
}

// TestBuildUserLabels_DualWrite_CheckOverridesProbe_BothSlots: probe env=dev, check env=prod.
// Both the un-prefixed and prefixed slots should have value "prod".
func TestBuildUserLabels_DualWrite_CheckOverridesProbe_BothSlots(t *testing.T) {
	result := buildUserLabels(
		labels("env", "dev"),
		labels("env", "prod"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	require.Len(t, result, 2)
	unprefixed := findLabel(t, result, "env")
	assert.Equal(t, "prod", unprefixed.value, "un-prefixed slot should have check value")
	prefixed := findLabel(t, result, "label_env")
	assert.Equal(t, "prod", prefixed.value, "prefixed slot should have check value")
}

// TestBuildUserLabels_DualWrite_ReservedDropped: the reserved label "probe" is dropped.
// The non-reserved label "env" still appears in both forms.
func TestBuildUserLabels_DualWrite_ReservedDropped(t *testing.T) {
	result := buildUserLabels(
		nil,
		labels("probe", "myprobe", "env", "prod"),
		sm.LabelMode_LABEL_MODE_DUAL_WRITE,
	)
	assertNoLabel(t, result, "probe")
	assertNoLabel(t, result, "label_probe")
	findLabel(t, result, "env")
	findLabel(t, result, "label_env")
	assert.Len(t, result, 2)
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

// TestBuildUserLabels_Unprefixed_ReservedDropped: reserved label "probe" is dropped.
func TestBuildUserLabels_Unprefixed_ReservedDropped(t *testing.T) {
	result := buildUserLabels(
		nil,
		labels("probe", "myprobe", "env", "prod"),
		sm.LabelMode_LABEL_MODE_UNPREFIXED,
	)
	assertNoLabel(t, result, "probe")
	assertNoLabel(t, result, "label_probe")
	require.Len(t, result, 1)
	assert.Equal(t, "env", result[0].name)
	assert.Equal(t, "prod", result[0].value)
}
