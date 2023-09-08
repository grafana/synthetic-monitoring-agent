package model

import (
	"testing"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/require"
)

func TestGetLocalAndRegionIDs(t *testing.T) {
	type expected struct {
		localID  int64
		regionID int
	}

	testcases := map[string]struct {
		input    GlobalID
		expected expected
	}{
		"local id": {
			input:    1,
			expected: expected{localID: 1, regionID: 0},
		},
		"min local id, min region id": {
			input:    localToGlobal(t, sm.MinLocalID, sm.MinRegionID),
			expected: expected{localID: sm.MinLocalID, regionID: sm.MinRegionID},
		},
		"min local id, max region id": {
			input:    localToGlobal(t, sm.MinLocalID, sm.MaxRegionID),
			expected: expected{localID: sm.MinLocalID, regionID: sm.MaxRegionID},
		},
		"max local id, min region id": {
			input:    localToGlobal(t, sm.MaxLocalID, sm.MinRegionID),
			expected: expected{localID: sm.MaxLocalID, regionID: sm.MinRegionID},
		},
		"max local id, max region id": {
			input:    localToGlobal(t, sm.MaxLocalID, sm.MaxRegionID),
			expected: expected{localID: sm.MaxLocalID, regionID: sm.MaxRegionID},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			localID, regionID := GetLocalAndRegionIDs(tc.input)
			require.Equal(t, tc.expected.localID, localID)
			require.Equal(t, tc.expected.regionID, regionID)
		})
	}
}

func TestCheckFromSM(t *testing.T) {
	var (
		testRid       = sm.MinRegionID
		testCid       = int64(sm.MinLocalID)
		testTid       = int64(sm.MinLocalID + 1)
		testGlobalCid = int64(localToGlobal(t, testCid, testRid))
		testGlobalTid = int64(localToGlobal(t, testTid, testRid))
	)

	type expected struct {
		check Check
	}

	testcases := map[string]struct {
		input    sm.Check
		expected expected
	}{
		"empty check": {
			input:    sm.Check{},
			expected: expected{check: Check{}},
		},
		"local ids": {
			input: sm.Check{
				Id:       testCid,
				TenantId: testTid,
				Settings: sm.CheckSettings{
					Ping: &sm.PingSettings{},
				},
			},
			expected: expected{
				check: Check{
					Check: sm.Check{
						Id:       testCid,
						TenantId: testTid,
						Settings: sm.CheckSettings{
							Ping: &sm.PingSettings{},
						},
					},
					RegionId: 0,
				},
			},
		},
		"global ids": {
			input: sm.Check{
				Id:       testGlobalCid,
				TenantId: testGlobalTid,
				Settings: sm.CheckSettings{
					Ping: &sm.PingSettings{},
				},
			},
			expected: expected{
				check: Check{
					Check: sm.Check{
						Id:       testCid,
						TenantId: testTid,
						Settings: sm.CheckSettings{
							Ping: &sm.PingSettings{},
						},
					},
					RegionId: testRid,
				},
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var c Check
			require.NoError(t, c.FromSM(tc.input))

			require.Equal(t, tc.expected.check, c)
			require.Equal(t, GlobalID(tc.input.Id), c.GlobalID())
			require.Equal(t, GlobalID(tc.input.TenantId), c.GlobalTenantID())
		})
	}
}

func localToGlobal(t *testing.T, localID int64, regionID int) GlobalID {
	t.Helper()
	globalID, err := sm.LocalIDToGlobalID(localID, regionID)
	require.NoError(t, err)
	return GlobalID(globalID)
}
