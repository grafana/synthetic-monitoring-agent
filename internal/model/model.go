package model

import (
	"fmt"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

type GlobalID int64

type Check struct {
	sm.Check
	RegionId int `json:"regionId"`
}

func (c *Check) FromSM(check sm.Check) error {
	// This implementation is a bit wasteful, but it ensures that it
	// remains in sync with the protobuf definition.

	data, err := check.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal check %d tenant %d: %w", check.Id, check.TenantId, err)
	}

	if err := c.Check.Unmarshal(data); err != nil {
		return fmt.Errorf("failed to unmarshal data for check %d tenant %d: %w", check.Id, check.TenantId, err)
	}

	cid, crid := GetLocalAndRegionIDs(GlobalID(check.Id))
	tid, trid := GetLocalAndRegionIDs(GlobalID(check.TenantId))

	if crid != trid {
		// This should never happen.
		return fmt.Errorf("inconsistent region ids %d and %d, checkId %d, tenantId %d", crid, trid, check.Id, check.TenantId)
	}

	c.Id = cid
	c.TenantId = tid
	c.RegionId = crid

	return nil
}

func (c *Check) GlobalID() GlobalID {
	id, err := sm.LocalIDToGlobalID(c.Id, c.RegionId)
	if err != nil {
		return GlobalID(c.Id)
	}
	return GlobalID(id)
}

func (c *Check) GlobalTenantID() GlobalID {
	id, err := sm.LocalIDToGlobalID(c.TenantId, c.RegionId)
	if err != nil {
		return GlobalID(c.TenantId)
	}
	return GlobalID(id)
}

// GetLocalAndRegionIDs takes a Global ID as specified in the sm data
// structures and returns a pair of ids corresponding to the local ID and the
// region ID. If the provided id is already a local one, it's returned without
// modification with the region set to 0.
func GetLocalAndRegionIDs(id GlobalID) (localID int64, regionID int) {
	localID, regionID, err := sm.GlobalIDToLocalID(int64(id))
	if err != nil {
		// Id is already local, use region 0.
		return int64(id), 0
	}
	return localID, regionID
}
