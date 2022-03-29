package testdata

import (
	. "github.com/coder/coder/coderd/authz"
)

type level string

const (
	otherOption = "other"

	levelWild   level = "*"
	levelSite   level = "site"
	levelOrg    level = "org"
	levelOrgMem level = "org:mem"
	// levelOrgAll is a helper to get both org levels above
	levelOrgAll level = "org:*"
	levelUser   level = "user"
)

var (
	PermissionTypes = []bool{true, false}
	Levels          = PermissionLevels
	LevelIDs        = []string{"", "mem"}
	ResourceTypes   = []string{"resource", "*", otherOption}
	ResourceIDs     = []string{"rid", "*", otherOption}
	Actions         = []string{"action", "*", otherOption}
)

func AllPermissions() Set {
	all := make(Set, 0, 2*len(Levels)*len(LevelIDs)*len(ResourceTypes)*len(ResourceIDs)*len(Actions))
	for _, p := range PermissionTypes {
		for _, l := range Levels {
			for _, lid := range LevelIDs {
				for _, t := range ResourceTypes {
					for _, i := range ResourceIDs {
						for _, a := range Actions {
							all = append(all, &Permission{
								Sign:         p,
								Level:        l,
								LevelID:      lid,
								ResourceType: t,
								ResourceID:   i,
								Action:       a,
							})
						}
					}
				}
			}
		}
	}
	return all
}
