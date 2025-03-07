// Copyright (C) 2025 Grafana Labs.
// SPDX-License-Identifier: AGPL-3.0-only

package version

import (
	"runtime/debug"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAll validates that the various function are returning _something_.
func TestAll(t *testing.T) {
	t.Parallel()

	const (
		versionValue  = "(devel)"
		revisionValue = "test-revision"
		timeValue     = "test-time"
	)

	// Override the getBuildInfo variable to ensure that we can test the
	// functions without having to rely on the actual build information.
	//
	// Before Go 1.24, test binaries would get their buildinfo populated in
	// the same way as regular packages. Starting with 1.24, that
	// information is no longer available, so the only possible test is to
	// ensure that the functions return _something_. This is basically
	// another instance of Hyrum's Law.
	//
	// Note that instead of simply returning a fixed value for the
	// buildinfo, this modifies the actual value returned by
	// debug.ReadBuildInfo. In this way if the behavior changes again, e.g.
	// it starts to return a value when running as part of a test, the test
	// will fail and we can decide what to do with that.
	//
	// See https://github.com/golang/go/issues/33976, sort of.
	getBuildInfo = sync.OnceValue(func() *debug.BuildInfo {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return nil
		}

		slices.SortFunc(bi.Settings, cmpBuildSettings)

		if _, found := slices.BinarySearchFunc(bi.Settings, revisionKey, isKey); !found {
			bi.Settings = append(bi.Settings, debug.BuildSetting{Key: revisionKey, Value: revisionValue})
		}

		if _, found := slices.BinarySearchFunc(bi.Settings, timeKey, isKey); !found {
			bi.Settings = append(bi.Settings, debug.BuildSetting{Key: timeKey, Value: timeValue})
		}

		slices.SortFunc(bi.Settings, cmpBuildSettings)

		return bi
	})

	require.Equal(t, versionValue, Short())
	require.Equal(t, revisionValue, Commit())
	require.Equal(t, timeValue, Buildstamp())
	require.NotEmpty(t, UserAgent())
}
