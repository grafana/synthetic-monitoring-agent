// Copyright 2022 Grafana Labs
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package synthetic_monitoring

import (
	"fmt"
	"math"
)

// This file contains methods for converting IDs for Synthetic Monitoring
// objects (checks, tenants) from single-region (local) to region-aware IDs
// (global). This is needed for agents that run checks for multiple
// regions.
//
// As IDs are only unique within their region, we create a new space
// of IDs (global IDs) that avoids collisions when objects from
// different regions are handled.
//
// At the same time, it is necessary to undo the process and obtain
// the region and local ID from a global ID, so that results can be
// associated with their respective regions.
//
// How this works:
//
// Local IDs are positive, non-zero integers, assigned sequentially.
//
// Global IDs are negative, non-zero integers. This allows to tell them
// apart from Local IDs easily and for both to coexist with some safety.
// They are constructed by multiplying the original ID by 1000 (MaxRegions)
// and then adding a unique regionID (<1000).
//
// For example, check with ID 1234 in region 3 will have a global ID of
// -1234003.
//
// This reduces the space of IDs available by a factor of 1000, from 63 bits
// to 53, which is still more than enough.
const (
	// MaxRegions is the maximum number of regions supported.
	MaxRegions = 1000

	// MinRegionID is the minimum valid region ID.
	MinRegionID = 1

	// MaxRegionID is the maximum valid region ID.
	MaxRegionID = MaxRegions - 1

	// BadID is the ID value that is not valid in any case
	// (as global, local or region ID).
	BadID = 0

	// MinLocalID is the smallest local ID, as 0 is not valid.
	MinLocalID = 1

	// MaxLocalID is the maximum value allowed for a local ID.
	// This is the largest positive integer that can be multiplied
	// by 1000 and 999 added to it and still fit in an int64.
	// MaxLocalID = 9_223_372_036_854_774
	MaxLocalID = (math.MaxInt64 / MaxRegions) - 1

	// MaxGlobalID is the maximum value a global ID can hold.
	// It is the equivalent to (MinLocalID, MinRegionID)
	// MaxGlobalID = -1001
	MaxGlobalID = -(MinLocalID*MaxRegions + MinRegionID)

	// MinGlobalID is the minimum value a GlobalID can hold.
	// MinGlobalID = -9_223_372_036_854_774_999
	MinGlobalID = -(MaxLocalID*MaxRegions + MaxRegionID)
)

// IsGlobalIDValid returns true if an ID is Global, false otherwise.
func IsGlobalIDValid(id int64) bool {
	return MinGlobalID <= id && id <= MaxGlobalID
}

// IsRegionIDValid checks that a region ID is within bounds.
func IsRegionIDValid(id int) bool {
	return MinRegionID <= id && id <= MaxRegionID
}

func IsLocalIDValid(id int64) bool {
	return MinLocalID <= id && id <= MaxLocalID
}

// LocalIDToGlobalID converts the given localID to a global ID
// using the given region ID.
func LocalIDToGlobalID(localID int64, regionID int) (int64, error) {
	if !IsLocalIDValid(localID) {
		return BadID, BadLocalIDError(localID)
	}
	if !IsRegionIDValid(regionID) {
		return BadID, BadRegionIDError(regionID)
	}
	return -(localID*MaxRegions + int64(regionID)), nil
}

// GlobalIDToLocalID converts a globalID back to a (local ID, region ID) pair.
func GlobalIDToLocalID(globalID int64) (localID int64, regionID int, err error) {
	if !IsGlobalIDValid(globalID) {
		return BadID, BadID, BadGlobalIDError(globalID)
	}
	localID = -globalID / MaxRegions
	regionID = int(-globalID % MaxRegions)
	if !IsRegionIDValid(regionID) {
		return BadID, BadID, BadGlobalIDError(globalID)
	}
	return localID, regionID, nil
}

// BadRegionIDError type is returned when an invalid region ID is used.
type BadRegionIDError int

// ID returns the ID that caused the error.
func (n BadRegionIDError) ID() int {
	return int(n)
}

// Error implements the error interface.
func (n BadRegionIDError) Error() string {
	return fmt.Sprintf("bad region ID: %d", n.ID())
}

// BadLocalIDError type is returned when an invalid local ID is used.
type BadLocalIDError int64

// ID returns the ID that caused the error.
func (n BadLocalIDError) ID() int64 {
	return int64(n)
}

// Error implements the error interface.
func (n BadLocalIDError) Error() string {
	return fmt.Sprintf("bad local ID: %d", n.ID())
}

// BadGlobalIDError type is returned when an invalid global ID is used.
type BadGlobalIDError int64

// ID returns the ID that caused the error.
func (n BadGlobalIDError) ID() int64 {
	return int64(n)
}

// Error implements the error interface.
func (n BadGlobalIDError) Error() string {
	return fmt.Sprintf("bad global ID: %d", n.ID())
}
