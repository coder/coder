package authztest

import (
	"github.com/coder/coder/coderd/authz"
)

const (
	OtherOption    = "other"
	PermObjectType = "resource"
	PermAction     = "read"
	PermOrgID      = "mem"
	PermObjectID   = "rid"
	PermMe         = "me"
)

var (
	levelIDs      = []string{"", PermOrgID}
	resourceTypes = []authz.ResourceType{PermObjectType, "*", OtherOption}
	resourceIDs   = []string{PermObjectID, "*", OtherOption}
	actions       = []authz.Action{PermAction, "*", OtherOption}
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
								Negate:         s,
								Level:          l,
								OrganizationID: PermOrgID,
								ResourceType:   t,
								ResourceID:     i,
								Action:         a,
							})
						}
						all = append(all, &authz.Permission{
							Negate:         s,
							Level:          l,
							OrganizationID: "",
							ResourceType:   t,
							ResourceID:     i,
							Action:         a,
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
	if p.ResourceType == OtherOption ||
		p.ResourceID == OtherOption ||
		p.Action == OtherOption {
		return SetNeutral
	}
	if p.Negate {
		return SetNegative
	}
	return SetPositive
}
