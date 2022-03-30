package authztest

import (
	. "github.com/coder/coder/coderd/authz"
)

const (
	otherOption = "other"
)

var (
	Levels        = PermissionLevels
	LevelIDs      = []string{"", "mem"}
	ResourceTypes = []string{"resource", "*", otherOption}
	ResourceIDs   = []string{"rid", "*", otherOption}
	Actions       = []string{"action", "*", otherOption}
)

// AllPermissions returns all the possible permissions ever.
func AllPermissions() Set {
	permissionTypes := []bool{true, false}
	all := make(Set, 0, len(permissionTypes)*len(Levels)*len(LevelIDs)*len(ResourceTypes)*len(ResourceIDs)*len(Actions))
	for _, s := range permissionTypes {
		for _, l := range Levels {
			for _, t := range ResourceTypes {
				for _, i := range ResourceIDs {
					for _, a := range Actions {
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
