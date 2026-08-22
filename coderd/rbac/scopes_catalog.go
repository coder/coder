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

	// Users
	"user:read":            {},
	"user:read_personal":   {},
	"user:update_personal": {},
	"user:*":               {},

	// User secrets
	"user_secret:read":   {},
	"user_secret:create": {},
	"user_secret:update": {},
	"user_secret:delete": {},
	"user_secret:*":      {},

	// User skills
	"user_skill:read":   {},
	"user_skill:create": {},
	"user_skill:update": {},
	"user_skill:delete": {},
	"user_skill:*":      {},

	// User memories
	"user_memory:read":   {},
	"user_memory:create": {},
	"user_memory:update": {},
	"user_memory:delete": {},
	"user_memory:*":      {},

	// Tasks
	"task:create": {},
	"task:read":   {},
	"task:update": {},
	"task:delete": {},
	"task:*":      {},

	// Organizations
	"organization:read":   {},
	"organization:update": {},
	"organization:delete": {},
	"organization:*":      {},
}

// Public composite coder:* scopes exposed to users.
var externalComposite = map[ScopeName]struct{}{
	"coder:workspaces.create":   {},
	"coder:workspaces.operate":  {},
	"coder:workspaces.delete":   {},
	"coder:workspaces.access":   {},
	"coder:templates.build":     {},
	"coder:templates.author":    {},
	"coder:apikeys.manage_self": {},
}

// scopeAliases maps the spellings accepted for backward compatibility onto the
// names the api_key_scope enum stores. IsExternalScope accepts every key and
// CanonicalScopeName rewrites it to its value, so the two agree by reading one
// table rather than by keeping two switches in step. Drift between them is
// worse in one direction than the other: a name accepted as public but not
// rewritten is declared requestable and then fails to expand on every request
// naming it.
var scopeAliases = map[ScopeName]ScopeName{
	"all":                 ScopeAll,
	"application_connect": ScopeApplicationConnect,
}

// IsExternalScope returns true if the scope is public: the `all` and
// `application_connect` aliases, the canonical `coder:all` and
// `coder:application_connect`, a curated low-level resource:action scope, or a
// curated composite `coder:*` scope.
func IsExternalScope(name ScopeName) bool {
	if _, ok := scopeAliases[name]; ok {
		return true
	}
	switch name {
	case ScopeAll, ScopeApplicationConnect:
		return true
	}
	if _, ok := externalLowLevel[name]; ok {
		return true
	}
	if _, ok := externalComposite[name]; ok {
		return true
	}

	return false
}

// CanonicalScopeName maps the backward-compatibility aliases IsExternalScope
// accepts onto the names the api_key_scope enum stores. Any other name is
// returned unchanged.
//
// IsExternalScope answers whether a name may be requested; it does not answer
// how that name is spelled once persisted. The aliases `all` and
// `application_connect` are accepted but are not enum members, so a caller
// that stores what it validated must canonicalize in between.
func CanonicalScopeName(name ScopeName) ScopeName {
	if canonical, ok := scopeAliases[name]; ok {
		return canonical
	}
	return name
}

// ExternalScopeNames returns a sorted list of all public scopes: the canonical
// `coder:all` and `coder:application_connect` spellings, the curated low-level
// resource:action names, and the curated composite coder:* scopes.
//
// Every name returned is canonical, so the list omits the bare `all` and
// `application_connect` aliases IsExternalScope also accepts. A caller matching
// a client-supplied name against this list must run it through
// CanonicalScopeName first, or reject a spelling the same package calls public.
func ExternalScopeNames() []string {
	names := make([]string, 0, len(externalLowLevel)+len(externalComposite)+2)
	names = append(names, string(ScopeAll))
	names = append(names, string(ScopeApplicationConnect))

	// curated low-level names, filtered for validity
	for name := range externalLowLevel {
		if _, _, ok := parseLowLevelScope(name); ok {
			names = append(names, string(name))
		}
	}

	// curated composite names
	for name := range externalComposite {
		names = append(names, string(name))
	}

	sort.Slice(names, func(i, j int) bool { return strings.Compare(names[i], names[j]) < 0 })
	return names
}
