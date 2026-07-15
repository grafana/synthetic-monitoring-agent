package backfill_test

import (
	"os"
	"sort"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

// fixtureMetricNames parses a scraper-fixture file (Prometheus text
// exposition format, e.g. internal/scraper/testdata/<type>.txt) and returns
// the sorted set of fully-expanded series names it declares: gauge/counter
// families as-is, and histogram/summary families expanded into their
// "_bucket"/"_sum"/"_count" series (matching how they appear as __name__
// label values on the TimeSeries CollectTyped emits). Used by the
// fixture-superset gate shared across A2-A6: build a generator for a check
// of the type under test, run CollectTyped with a successful sample, extract
// its series names, and assert set equality against this helper's output for
// the corresponding fixture.
func fixtureMetricNames(t *testing.T, path string) []string {
	t.Helper()

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(f)
	require.NoError(t, err)

	names := make(map[string]struct{}, len(families))
	for name, mf := range families {
		switch mf.GetType() {
		case dto.MetricType_HISTOGRAM:
			names[name+"_bucket"] = struct{}{}
			names[name+"_sum"] = struct{}{}
			names[name+"_count"] = struct{}{}
		case dto.MetricType_SUMMARY:
			names[name+"_sum"] = struct{}{}
			names[name+"_count"] = struct{}{}
		default:
			names[name] = struct{}{}
		}
	}

	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
