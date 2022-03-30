package authztest

import "github.com/coder/coder/coderd/authz"

type level string

const (
	levelWild   level = "level-wild"
	levelSite   level = "level-site"
	levelOrg    level = "level-org"
	levelOrgMem level = "level-org:mem"
	// levelOrgAll is a helper to get both org levels above
	levelOrgAll level = "level-org:*"
	levelUser   level = "level-user"
)

// LevelGroup is all permissions for a given level
type LevelGroup map[PermissionSet]Set

func (lg LevelGroup) All() Set {
	pos := lg.Positive()
	neg := lg.Negative()
	net := lg.Abstain()
	all := make(Set, len(pos)+len(neg)+len(net))
	var i int
	i += copy(all[i:], pos)
	i += copy(all[i:], neg)
	i += copy(all[i:], net)
	return all
}

func (lg LevelGroup) Positive() Set {
	return lg[SetPositive]
}

func (lg LevelGroup) Negative() Set {
	return lg[SetNegative]
}

func (lg LevelGroup) Abstain() Set {
	return lg[SetNeutral]
}

func GroupedPermissions(perms Set) SetGroup {
	groups := make(SetGroup)
	allLevelKeys := []level{levelWild, levelSite, levelOrg, levelOrgMem, levelOrgAll, levelUser}

	for _, l := range allLevelKeys {
		groups[l] = make(LevelGroup)
	}

	for _, p := range perms {
		m := Impact(p)
		switch {
		case p.Level == authz.LevelSite:
			groups[levelSite][m] = append(groups[levelSite][m], p)
		case p.Level == authz.LevelOrg:
			groups[levelOrgAll][m] = append(groups[levelOrgAll][m], p)
			if p.LevelID == "" || p.LevelID == "*" {
				groups[levelOrg][m] = append(groups[levelOrg][m], p)
			} else {
				groups[levelOrgMem][m] = append(groups[levelOrgMem][m], p)
			}
		case p.Level == authz.LevelUser:
			groups[levelUser][m] = append(groups[levelUser][m], p)
		case p.Level == authz.LevelWildcard:
			groups[levelWild][m] = append(groups[levelWild][m], p)
		}
	}

	return groups
}

type SetGroup map[level]LevelGroup

func (s SetGroup) Wildcard() LevelGroup {
	return s[levelWild]
}

func (s SetGroup) Site() LevelGroup {
	return s[levelSite]
}

func (s SetGroup) Org() LevelGroup {
	return s[levelOrg]
}

func (s SetGroup) AllOrgs() LevelGroup {
	return s[levelOrgAll]
}

func (s SetGroup) OrgMem() LevelGroup {
	return s[levelOrgMem]
}

func (s SetGroup) User() LevelGroup {
	return s[levelUser]
}
