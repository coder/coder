package authztest

import (
	. "github.com/coder/coder/coderd/authz"
)

const (
	otherOption = "other"
)

var (
	levelIDs      = []string{"", "mem"}
	resourceTypes = []string{"resource", "*", otherOption}
	resourceIDs   = []string{"rid", "*", otherOption}
	actions       = []string{"action", "*", otherOption}
)

// AllPermissions returns all the possible permissions ever.
func AllPermissions() Set {
	permissionTypes := []bool{true, false}
	all := make(Set, 0, len(permissionTypes)*len(PermissionLevels)*len(levelIDs)*len(resourceTypes)*len(resourceIDs)*len(actions))
	for _, s := range permissionTypes {
		for _, l := range PermissionLevels {
			for _, t := range resourceTypes {
				for _, i := range resourceIDs {
					for _, a := range actions {
						if l == LevelOrg {
							all = append(all, &Permission{
								Sign:         s,
								Level:        l,
								LevelID:      "mem",
								ResourceType: t,
								ResourceID:   i,
								Action:       a,
							})
						}
						all = append(all, &Permission{
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
func Impact(p *Permission) PermissionSet {
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
