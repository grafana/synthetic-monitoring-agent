// Package accounting provides information about the number of active
// series produed by checks and other related metrics.
package accounting

import (
	"errors"
	"strings"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

//go:generate ./generate-active-series-counts

var ErrUnhandledCheck = errors.New("cannot compute the number of active series for check")

// GetActiveSeriesForCheck returns the number of active series that the
// provided check produces. The data is embedded in the program at build
// time and it's obtained from the test data used to keep the generated
// series in sync with the rest of the program.
//
// This is of course dependent on the running agent being the same
// version as the code embedded in the program using this information.
func GetActiveSeriesForCheck(check synthetic_monitoring.Check) (int, error) {
	checkType := check.Type()
	key := checkType.String()

	switch checkType {
	case synthetic_monitoring.CheckTypeDns:

	case synthetic_monitoring.CheckTypeHttp:
		if strings.HasPrefix(check.Target, "https://") {
			key += "_ssl"
		}

	case synthetic_monitoring.CheckTypePing:

	case synthetic_monitoring.CheckTypeTcp:
		if check.Settings.Tcp.Tls {
			key += "_ssl"
		}

	default:
		return 0, ErrUnhandledCheck
	}

	if check.BasicMetricsOnly {
		key += "_basic"
	}

	as, found := activeSeriesByCheckType[key]
	if !found {
		return 0, ErrUnhandledCheck
	}

	return as, nil
}
