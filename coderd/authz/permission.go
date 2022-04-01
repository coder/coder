package authz

import "strings"

type permLevel string

const (
	LevelWildcard permLevel = "*"
	LevelSite     permLevel = "site"
	LevelOrg      permLevel = "org"
	LevelUser     permLevel = "user"
)

var PermissionLevels = [4]permLevel{LevelWildcard, LevelSite, LevelOrg, LevelUser}

type Permission struct {
	// Sign is positive or negative.
	// True = Positive, False = negative
	Sign  bool
	Level permLevel
	// LevelID is used for identifying a particular org.
	//	org:1234
	LevelID string

	ResourceType ResourceType
	ResourceID   string
	Action       Action
}

// String returns the <level>.<resource_type>.<id>.<action> string formatted permission.
// A string builder is used to be the most efficient.
func (p Permission) String() string {
	var s strings.Builder
	// This could be 1 more than the actual capacity. But being 1 byte over for capacity is ok.
	s.Grow(1 + 4 + len(p.Level) + len(p.LevelID) + len(p.ResourceType) + len(p.ResourceID) + len(p.Action))
	if p.Sign {
		s.WriteRune('+')
	} else {
		s.WriteRune('-')
	}
	s.WriteString(string(p.Level))
	if p.LevelID != "" {
		s.WriteRune(':')
		s.WriteString(p.LevelID)
	}
	s.WriteRune('.')
	s.WriteString(string(p.ResourceType))
	s.WriteRune('.')
	s.WriteString(p.ResourceID)
	s.WriteRune('.')
	s.WriteString(string(p.Action))
	return s.String()
}
