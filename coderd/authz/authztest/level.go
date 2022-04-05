package authztest

import "github.com/coder/coder/coderd/authz"

type level string

const (
	LevelWildKey   level = "level-wild"
	LevelSiteKey   level = "level-site"
	LevelOrgKey    level = "level-org"
	LevelOrgMemKey level = "level-org:mem"
	// LevelOrgAllKey is a helper to get both org levels above
	LevelOrgAllKey level = "level-org:*"
	LevelUserKey   level = "level-user"
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
	copy(all[i:], net)
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
	allLevelKeys := []level{LevelWildKey, LevelSiteKey, LevelOrgKey, LevelOrgMemKey, LevelOrgAllKey, LevelUserKey}

	for _, l := range allLevelKeys {
		groups[l] = make(LevelGroup)
	}

	for _, p := range perms {
		m := Impact(p)
		switch {
		case p.Level == authz.LevelSite:
			groups[LevelSiteKey][m] = append(groups[LevelSiteKey][m], p)
		case p.Level == authz.LevelOrg:
			groups[LevelOrgAllKey][m] = append(groups[LevelOrgAllKey][m], p)
			if p.OrganizationID == "" || p.OrganizationID == "*" {
				groups[LevelOrgKey][m] = append(groups[LevelOrgKey][m], p)
			} else {
				groups[LevelOrgMemKey][m] = append(groups[LevelOrgMemKey][m], p)
			}
		case p.Level == authz.LevelUser:
			groups[LevelUserKey][m] = append(groups[LevelUserKey][m], p)
		case p.Level == authz.LevelWildcard:
			groups[LevelWildKey][m] = append(groups[LevelWildKey][m], p)
		}
	}

	return groups
}

type SetGroup map[level]LevelGroup

func (s SetGroup) Wildcard() LevelGroup {
	return s[LevelWildKey]
}

func (s SetGroup) Site() LevelGroup {
	return s[LevelSiteKey]
}

func (s SetGroup) Org() LevelGroup {
	return s[LevelOrgKey]
}

func (s SetGroup) AllOrgs() LevelGroup {
	return s[LevelOrgAllKey]
}

func (s SetGroup) OrgMem() LevelGroup {
	return s[LevelOrgMemKey]
}

func (s SetGroup) User() LevelGroup {
	return s[LevelUserKey]
}
