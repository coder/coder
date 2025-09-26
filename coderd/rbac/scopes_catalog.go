package rbac

import (
	"sort"
	"strings"
)

// externalLowLevel is the curated set of low-level scope names exposed to users.
// Any valid resource:action pair not in this set is considered internal-only
// and must not be user-requestable.
var externalLowLevel = map[ScopeName]struct{}{
	// Workspaces
	"workspace:read":                {},
	"workspace:create":              {},
	"workspace:update":              {},
	"workspace:delete":              {},
	"workspace:ssh":                 {},
	"workspace:start":               {},
	"workspace:stop":                {},
	"workspace:application_connect": {},
	"workspace:*":                   {},

	// Templates
	"template:read":   {},
	"template:create": {},
	"template:update": {},
	"template:delete": {},
	"template:use":    {},
	"template:*":      {},

	// API keys (self-management)
	"api_key:read":   {},
	"api_key:create": {},
	"api_key:update": {},
	"api_key:delete": {},
	"api_key:*":      {},

	// Files
	"file:read":   {},
	"file:create": {},
	"file:*":      {},

	// Users (personal profile only)
	"user:read_personal":   {},
	"user:update_personal": {},

	// User secrets
	"user_secret:read":   {},
	"user_secret:create": {},
	"user_secret:update": {},
	"user_secret:delete": {},
	"user_secret:*":      {},
}

// IsExternalScope returns true if the scope is public, including the
// `all` and `application_connect` special scopes and the curated
// low-level resource:action scopes.
func IsExternalScope(name ScopeName) bool {
	switch name {
	// Include `all` and `application_connect` for backward compatibility.
	case "all", ScopeAll, "application_connect", ScopeApplicationConnect:
		return true
	}
	if _, ok := externalLowLevel[name]; ok {
		return true
	}

	return false
}

// ExternalScopeNames returns a sorted list of all public scopes, which includes
// the `all` and `application_connect` special scopes and the curated public
// low-level names.
func ExternalScopeNames() []string {
	names := make([]string, 0, len(externalLowLevel)+2)
	names = append(names, string(ScopeAll))
	names = append(names, string(ScopeApplicationConnect))

	// curated low-level names, filtered for validity
	for name := range externalLowLevel {
		if _, _, ok := parseLowLevelScope(name); ok {
			names = append(names, string(name))
		}
	}

	sort.Slice(names, func(i, j int) bool { return strings.Compare(names[i], names[j]) < 0 })
	return names
}
