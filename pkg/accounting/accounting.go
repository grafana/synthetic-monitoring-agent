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
	accountingClass, err := GetCheckAccountingClass(check)
	if err != nil {
		return 0, err
	}

	as, found := activeSeriesByCheckType[accountingClass]
	if !found {
		return 0, ErrUnhandledCheck
	}

	return as, nil
}

// GetCheckAccountingClass returns the accounting class corresponding to
// the specified check.
func GetCheckAccountingClass(check synthetic_monitoring.Check) (string, error) {
	checkType := check.Type()
	key := checkType.String()

	switch checkType {
	case synthetic_monitoring.CheckTypeDns:

	case synthetic_monitoring.CheckTypeHttp:
		if strings.HasPrefix(check.Target, "https://") {
			key += "_ssl"
		}

	case synthetic_monitoring.CheckTypeScripted, synthetic_monitoring.CheckTypeBrowser:

	case synthetic_monitoring.CheckTypeMultiHttp:

	case synthetic_monitoring.CheckTypePing:

	case synthetic_monitoring.CheckTypeTcp:
		if check.Settings.Tcp.Tls {
			key += "_ssl"
		}

	case synthetic_monitoring.CheckTypeTraceroute:

	case synthetic_monitoring.CheckTypeGrpc:
		if check.Settings.Grpc.Tls {
			key += "_ssl"
		}

	default:
		return "", ErrUnhandledCheck
	}

	if check.BasicMetricsOnly {
		key += "_basic"
	}

	return key, nil
}

// ClassInfo contains information about a specific accounting class
type ClassInfo struct {
	CheckType  synthetic_monitoring.CheckType  // the correspodning check type for this class
	CheckClass synthetic_monitoring.CheckClass // the check class for this accounting class
	Series     int                             // how many series does this class of check produce
}

// GetAccountingClassInfo returns all the known accounting classes and
// their corresponding information
func GetAccountingClassInfo() map[string]ClassInfo {
	info := make(map[string]ClassInfo, len(activeSeriesByCheckType))

	for class, as := range activeSeriesByCheckType {
		checkType := getTypeFromClass(class)
		info[class] = ClassInfo{
			CheckType:  getTypeFromClass(class),
			CheckClass: checkType.Class(),
			Series:     as,
		}
	}

	return info
}

// getTypeFromClass is a helper that returns the corresponding check
// type for a given accounting class.
func getTypeFromClass(class string) synthetic_monitoring.CheckType {
	// We know that the accounting class is built by appending
	// variations to the base check type so strip that to get back
	// the check type.
	str := strings.SplitN(class, "_", 2)[0]
	checkType, found := synthetic_monitoring.CheckTypeFromString(str)
	if !found {
		panic("unhandled check type string")
	}

	return checkType
}
