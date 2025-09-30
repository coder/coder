package rbac

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// externalLowLevel is the curated set of low-level scope names exposed to users.
// Any valid resource:action pair not in this set is considered internal-only
// and must not be user-requestable.
type LowLevelScopeMeta struct {
	Name     ScopeName
	Resource string
	Action   policy.Action
}

type CompositeScopeMeta struct {
	Name      ScopeName
	ExpandsTo []ScopeName
}

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

	// User secrets
	"user_secret:read":   {},
	"user_secret:create": {},
	"user_secret:update": {},
	"user_secret:delete": {},
	"user_secret:*":      {},

	// Tasks
	"task:create": {},
	"task:read":   {},
	"task:update": {},
	"task:delete": {},
	"task:*":      {},
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

var externalSpecial = []ScopeName{ScopeAll, ScopeApplicationConnect}

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
	if _, ok := externalComposite[name]; ok {
		return true
	}

	return false
}

// ExternalScopeNames returns a sorted list of all public scopes, which
// includes the `all` and `application_connect` special scopes, curated
// low-level resource:action names, and curated composite coder:* scopes.
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

// ExternalLowLevelCatalog returns metadata for all public low-level scopes.
func ExternalLowLevelCatalog() []LowLevelScopeMeta {
	metas := make([]LowLevelScopeMeta, 0, len(externalLowLevel))
	for name := range externalLowLevel {
		resource, action, ok := parseLowLevelScope(name)
		if !ok {
			continue
		}
		metas = append(metas, LowLevelScopeMeta{
			Name:     name,
			Resource: resource,
			Action:   action,
		})
	}
	sort.Slice(metas, func(i, j int) bool {
		if metas[i].Resource == metas[j].Resource {
			return metas[i].Name < metas[j].Name
		}
		return metas[i].Resource < metas[j].Resource
	})
	return metas
}

// ExternalCompositeCatalog returns metadata for public composite coder:* scopes.
func ExternalCompositeCatalog() []CompositeScopeMeta {
	metas := make([]CompositeScopeMeta, 0, len(externalComposite))
	for name := range externalComposite {
		perms, ok := compositePerms[name]
		if !ok {
			continue
		}
		expands := make([]ScopeName, 0)
		for resource, actions := range perms {
			for _, action := range actions {
				expands = append(expands, ScopeName(fmt.Sprintf("%s:%s", resource, action)))
			}
		}
		slices.Sort(expands)
		metas = append(metas, CompositeScopeMeta{
			Name:      name,
			ExpandsTo: uniqueScopeNames(expands),
		})
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Name < metas[j].Name })
	return metas
}

// ExternalSpecialScopes returns the list of legacy/special public scopes that
// are not part of the low-level or composite catalogs but remain requestable
// for backward compatibility.
func ExternalSpecialScopes() []ScopeName {
	out := make([]ScopeName, len(externalSpecial))
	copy(out, externalSpecial)
	slices.Sort(out)
	return out
}

func uniqueScopeNames(in []ScopeName) []ScopeName {
	if len(in) == 0 {
		return nil
	}
	out := make([]ScopeName, 0, len(in))
	last := ScopeName("")
	for i, name := range in {
		if i == 0 || name != last {
			out = append(out, name)
			last = name
		}
	}
	return out
}
