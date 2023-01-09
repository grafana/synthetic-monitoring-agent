package synthetic_monitoring

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalIDToGlobalID(t *testing.T) {
	for name, test := range map[string]struct {
		localID  int64
		regionID int
		expected int64
		err      error
	}{
		"simple": {
			localID:  1234,
			regionID: 3,
			expected: -1234_003,
		},
		"min": {
			localID:  MinLocalID,
			regionID: MinRegionID,
			expected: MaxGlobalID,
		},
		"max": {
			localID:  MaxLocalID,
			regionID: MaxRegionID,
			expected: MinGlobalID,
		},
		"bad local ID": {
			localID:  BadID,
			regionID: 3,
			err:      BadLocalIDError(BadID),
		},
		"global as local": {
			localID:  -1234003,
			regionID: 1,
			err:      BadLocalIDError(-1234003),
		},
		"bad region ID": {
			localID:  1234,
			regionID: BadID,
			err:      BadRegionIDError(BadID),
		},
		"bad region ID 2": {
			localID:  1234,
			regionID: MaxRegionID + 1,
			err:      BadRegionIDError(MaxRegionID + 1),
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := LocalIDToGlobalID(test.localID, test.regionID)
			require.Equal(t, test.err, err)
			require.Equal(t, test.expected, result)
			if test.err == nil {
				require.True(t, IsLocalIDValid(test.localID))
				require.True(t, IsRegionIDValid(test.regionID))
				require.True(t, IsGlobalIDValid(result))
			} else {
				require.False(t, IsGlobalIDValid(result))
				require.False(t, IsLocalIDValid(test.localID) && IsRegionIDValid(test.regionID))
			}
		})
	}
}

func TestGlobalIDToLocalID(t *testing.T) {
	for name, test := range map[string]struct {
		globalID int64
		regionID int
		localID  int64
		err      error
	}{
		"simple": {
			globalID: -1234_003,
			localID:  1234,
			regionID: 3,
		},
		"min": {
			globalID: MaxGlobalID,
			localID:  MinLocalID,
			regionID: MinRegionID,
		},
		"max": {
			globalID: MinGlobalID,
			localID:  MaxLocalID,
			regionID: MaxRegionID,
		},
		"local as global": {
			globalID: 3,
			err:      BadGlobalIDError(3),
		},
		"invalid global": {
			globalID: -3000, // Region ID == 0
			err:      BadGlobalIDError(-3000),
		},
	} {
		t.Run(name, func(t *testing.T) {
			local, region, err := GlobalIDToLocalID(test.globalID)
			require.Equal(t, test.err, err)
			require.Equal(t, test.localID, local)
			require.Equal(t, test.regionID, region)
			if test.err == nil {
				require.True(t, IsGlobalIDValid(test.globalID))
				require.True(t, IsLocalIDValid(test.localID))
				require.True(t, IsRegionIDValid(test.regionID))
				gID, err2 := LocalIDToGlobalID(local, region)
				require.NoError(t, err2)
				require.Equal(t, test.globalID, gID)
			} else {
				require.False(t, IsLocalIDValid(test.localID))
				require.False(t, IsRegionIDValid(test.regionID))
			}
		})
	}
}
