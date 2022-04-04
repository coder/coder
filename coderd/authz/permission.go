package authz

import (
	"fmt"
	"golang.org/x/xerrors"
	"strings"
)

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
func (p Permission) String() string {
	sign := "-"
	if p.Sign {
		sign = "+"
	}
	levelID := ""
	if p.LevelID != "" {
		levelID = ":" + p.LevelID
	}

	return fmt.Sprintf("%s%s%s.%s.%s.%s",
		sign, p.Level, levelID, p.ResourceType, p.ResourceID, p.Action)
}

func ParsePermissions(perms string) ([]Permission, error) {
	permList := strings.Split(perms, ",")
	parsed := make([]Permission, 0, len(permList))
	for _, permStr := range permList {
		p, err := ParsePermission(strings.TrimSpace(permStr))
		if err != nil {
			return nil, xerrors.Errorf("perm '%s': %w", permStr, err)
		}
		parsed = append(parsed, p)
	}
	return parsed, nil
}

func ParsePermission(perm string) (Permission, error) {
	parts := strings.Split(perm, ".")
	if len(parts) != 4 {
		return Permission{}, xerrors.Errorf("permission expects 4 parts, got %d", len(parts))
	}

	level, resType, resID, act := parts[0], parts[1], parts[2], parts[3]

	if len(level) < 2 {
		return Permission{}, xerrors.Errorf("permission level is too short: '%s'", parts[0])
	}
	sign := level[0]
	levelParts := strings.Split(level[1:], ":")
	if len(levelParts) > 2 {
		return Permission{}, xerrors.Errorf("unsupported level format")
	}

	var permission Permission

	switch sign {
	case '+':
		permission.Sign = true
	case '-':
	default:
		return Permission{}, xerrors.Errorf("sign must be +/-")
	}

	switch permLevel(strings.ToLower(levelParts[0])) {
	case LevelWildcard:
		permission.Level = LevelWildcard
	case LevelSite:
		permission.Level = LevelSite
	case LevelOrg:
		permission.Level = LevelOrg
	case LevelUser:
		permission.Level = LevelUser
	default:
		return Permission{}, xerrors.Errorf("'%s' is an unsupported level", levelParts[0])
	}

	if len(levelParts) > 1 {
		permission.LevelID = levelParts[1]
	}

	// might want to check if these are valid resource types and actions.
	permission.ResourceType = ResourceType(resType)
	permission.ResourceID = resID
	permission.Action = Action(act)
	return permission, nil
}
