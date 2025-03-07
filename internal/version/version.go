// Copyright (C) 2025 Grafana Labs.
// SPDX-License-Identifier: AGPL-3.0-only

package version

import (
	"fmt"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
)

const (
	revisionKey = "vcs.revision"
	timeKey     = "vcs.time"
	smHomepage  = "https://github.com/grafana/synthetic-monitoring-agent"
)

func Short() string {
	bi := getBuildInfo()

	return bi.Main.Version
}

func Commit() string {
	return getBuildInfoByKey(revisionKey)
}

func Buildstamp() string {
	return getBuildInfoByKey(timeKey)
}

func UserAgent() string {
	return userAgentStr()
}

func getBuildInfoByKey(key string) string {
	bi := getBuildInfo()
	if bi == nil {
		return "invalid"
	}

	idx, found := slices.BinarySearchFunc(bi.Settings, key, isKey)
	if found {
		return bi.Settings[idx].Value
	}

	return "unknown"
}

//nolint:gochecknoglobals // This variable is only accessed in this package.
var getBuildInfo = sync.OnceValue(func() *debug.BuildInfo {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}

	slices.SortFunc(bi.Settings, cmpBuildSettings)

	return bi
})

var userAgentStr = sync.OnceValue(func() string {
	bi := getBuildInfo()

	program := filepath.Base(bi.Path)

	return fmt.Sprintf("%s/%s (%s %s; %s; %s; +%s)", program, Short(), runtime.GOOS, runtime.GOARCH, Commit(), Buildstamp(), smHomepage)
})

func cmpBuildSettings(a, b debug.BuildSetting) int {
	if v := strings.Compare(a.Key, b.Key); v != 0 {
		return v
	}

	return strings.Compare(a.Value, b.Value)
}

func isKey(a debug.BuildSetting, key string) int {
	return strings.Compare(a.Key, key)
}
