package authztest

import (
	"github.com/coder/coder/coderd/authz"
)

const (
	otherOption    = "other"
	PermObjectType = "resource"
	PermAction     = "read"
	PermOrgID      = "mem"
	PermObjectID   = "rid"
	PermMe         = "me"
)

var (
	levelIDs      = []string{"", PermOrgID}
	resourceTypes = []string{PermObjectType, "*", otherOption}
	resourceIDs   = []string{PermObjectID, "*", otherOption}
	actions       = []string{PermAction, "*", otherOption}
)

// AllPermissions returns all the possible permissions ever.
func AllPermissions() Set {
	permissionTypes := []bool{true, false}
	all := make(Set, 0, len(permissionTypes)*len(authz.PermissionLevels)*len(levelIDs)*len(resourceTypes)*len(resourceIDs)*len(actions))
	for _, s := range permissionTypes {
		for _, l := range authz.PermissionLevels {
			for _, t := range resourceTypes {
				for _, i := range resourceIDs {
					for _, a := range actions {
						if l == authz.LevelOrg {
							all = append(all, &authz.Permission{
								Sign:         s,
								Level:        l,
								LevelID:      PermOrgID,
								ResourceType: t,
								ResourceID:   i,
								Action:       a,
							})
						}
						all = append(all, &authz.Permission{
							Sign:         s,
							Level:        l,
							LevelID:      "",
							ResourceType: t,
							ResourceID:   i,
							Action:       a,
						})
					}
				}
			}
		}
	}
	return all
}

// Impact returns the impact (positive, negative, abstain) of p
func Impact(p *authz.Permission) PermissionSet {
	if p.ResourceType == otherOption ||
		p.ResourceID == otherOption ||
		p.Action == otherOption {
		return SetNeutral
	}
	if p.Sign {
		return SetPositive
	}
	return SetNegative
}
