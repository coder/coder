package authztest

import "strings"

// Permission Sets

//type Permission [(270 / 8) + 1]byte

// level.resource.id.action
type Permission [5]int

func (p Permission) Type() permissionType {
	return PermissionTypes[p[0]]
}

func (p Permission) Level() level {
	return Levels[p[1]]
}

func (p Permission) ResourceType() resourceType {
	return ResourceTypes[p[2]]
}

func (p Permission) ResourceID() resourceID {
	return ResourceIDs[p[3]]
}

func (p Permission) Action() action {
	return Actions[p[4]]
}

func (p Permission) String() string {
	var s strings.Builder
	s.WriteString(string(p.Type()))
	s.WriteString(string(p.Level()))
	s.WriteRune('.')
	s.WriteString(string(p.ResourceType()))
	s.WriteRune('.')
	s.WriteString(string(p.ResourceID()))
	s.WriteRune('.')
	s.WriteString(string(p.Action()))
	return s.String()
}

type permissionSet string

const (
	SetPositive permissionSet = "j"
	SetNegative permissionSet = "j!"
	SetNeutral  permissionSet = "a"
)

var (
	PermissionSets = []permissionSet{SetPositive, SetNegative, SetNeutral}
)

// TSet returns what set the permission is included in
func (p Permission) Set() permissionSet {
	if p.ResourceType() == otherOption ||
		p.ResourceID() == otherOption ||
		p.Action() == otherOption {
		return SetNeutral
	}
	if p.Type() == "+" {
		return SetPositive
	}
	return SetNegative
}

type permissionType string
type resourceType string
type resourceID string
type action string

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
	PermissionTypes = []permissionType{"+", "-"}
	Levels          = []level{levelWild, levelSite, levelOrg, levelOrgMem, levelUser}
	ResourceTypes   = []resourceType{"resource", "*", otherOption}
	ResourceIDs     = []resourceID{"rid", "*", otherOption}
	Actions         = []action{"action", "*", otherOption}
)

func AllPermissions() Set {
	all := make(Set, 0, 2*len(Levels)*len(ResourceTypes)*len(ResourceIDs)*len(Actions))
	for p := range PermissionTypes {
		for l := range Levels {
			for t := range ResourceTypes {
				for i := range ResourceIDs {
					for a := range Actions {
						all = append(all, &Permission{p, l, t, i, a})
					}
				}
			}
		}
	}
	return all
}

// LevelGroup is all permissions for a given level
type LevelGroup map[permissionSet]Set

func (lg LevelGroup) All() Set {
	pos := lg.Positive()
	neg := lg.Negative()
	net := lg.Abstain()
	all := make(Set, len(pos)+len(neg)+len(net))
	var i int
	i += copy(all, pos)
	i += copy(all, neg)
	i += copy(all, net)
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

func GroupedPermissions(perms Set) setGroup {
	groups := make(setGroup)
	for _, l := range append(Levels, levelOrgAll) {
		groups[l] = make(LevelGroup)
	}

	for _, p := range perms {
		m := p.Set()
		l := p.Level()
		groups[l][m] = append(groups[l][m], p)
		if l == levelOrg || l == levelOrgMem {
			groups[levelOrgAll][m] = append(groups[levelOrgAll][m], p)
		}
	}

	return groups
}

type setGroup map[level]LevelGroup

func (s setGroup) Level(l level) LevelGroup {
	return s[l]
}

func (s setGroup) Wildcard() LevelGroup {
	return s[levelWild]
}
func (s setGroup) Site() LevelGroup {
	return s[levelSite]
}
func (s setGroup) Org() LevelGroup {
	return s[levelOrgMem]
}
func (s setGroup) AllOrgs() LevelGroup {
	return s[levelOrgAll]
}
func (s setGroup) OrgMem() LevelGroup {
	return s[levelOrgMem]
}
func (s setGroup) User() LevelGroup {
	return s[levelUser]
}

type Cache struct {
	setGroup
}

func NewCache(perms Set) *Cache {
	c := &Cache{
		setGroup: GroupedPermissions(perms),
	}
	return c
}
