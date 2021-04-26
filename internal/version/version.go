package version

import (
	"fmt"
	"path/filepath"
	"runtime"
	"runtime/debug"
)

const smHomepage = "https://github.com/grafana/synthetic-monitoring-agent"

var (
	version    = "unknown"
	commit     = "0000000000000000000000000000000000000000"
	buildstamp = "1970-01-01 00:00:00+00:00"
	userAgent  = buildUserAgentStr()
)

func Short() string {
	return version
}

func Commit() string {
	return commit
}

func Buildstamp() string {
	return buildstamp
}

func UserAgent() string {
	return userAgent
}

func buildUserAgentStr() string {
	program := "unknown"

	if info, ok := debug.ReadBuildInfo(); ok {
		program = filepath.Base(info.Path)
	}

	return fmt.Sprintf("%s/%s (%s %s; %s; %s; +%s)", program, version, runtime.GOOS, runtime.GOARCH, commit, buildstamp, smHomepage)
}
