package pusher

import (
	"reflect"
	"sort"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestNewMetrics(t *testing.T) {
	t.Run("non-nil fields", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		m := NewMetrics(reg)
		rVal := reflect.ValueOf(m)
		for i := 0; i < rVal.NumField(); i++ {
			fType := rVal.Type().Field(i)
			fVal := rVal.Field(i)
			if fVal.Kind() == reflect.Pointer {
				require.NotZero(t, fVal.Pointer(), fType.Name)
			}
		}
	})
	t.Run("registered fields", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		m := NewMetrics(reg).
			WithTenant(1234, 50).
			WithType(LabelValueMetrics)

		m.PushCounter.WithLabelValues().Inc()
		m.DroppedCounter.WithLabelValues().Inc()
		m.FailedCounter.WithLabelValues(LabelValueRetryExhausted).Inc()
		m.BytesOut.WithLabelValues().Add(1200)
		m.ErrorCounter.WithLabelValues("500").Inc()
		m.ResponseCounter.WithLabelValues("200").Inc()
		m.InstalledHandlers.Inc()

		fam, err := reg.Gather()
		require.NoError(t, err)
		var (
			expected = []string{
				"sm_agent_publisher_drop_total",
				"sm_agent_publisher_handlers_total",
				"sm_agent_publisher_push_bytes",
				"sm_agent_publisher_push_errors_total",
				"sm_agent_publisher_push_failed_total",
				"sm_agent_publisher_push_total",
				"sm_agent_publisher_responses_total",
			}
			actual []string
		)
		for _, metric := range fam {
			actual = append(actual, metric.GetName())
		}

		sort.Strings(expected)
		sort.Strings(actual)

		require.Equal(t, expected, actual)
	})
}
