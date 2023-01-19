package rbac

import (
	"fmt"

	"golang.org/x/xerrors"
)

type ScopeName string

// Scope acts the exact same as a Role with the addition that is can also
// apply an AllowIDList. Any resource being checked against a Scope will
// reject any resource that is not in the AllowIDList.
// To not use an AllowIDList to reject authorization, use a wildcard for the
// AllowIDList. Eg: 'AllowIDList: []string{WildcardSymbol}'
type Scope struct {
	Role
	AllowIDList []string `json:"allow_list"`
}

const (
	ScopeAll                ScopeName = "all"
	ScopeApplicationConnect ScopeName = "application_connect"
)

// TODO: Support passing in scopeID list for allowlisting resources.
var builtinScopes = map[ScopeName]Scope{
	// ScopeAll is a special scope that allows access to all resources. During
	// authorize checks it is usually not used directly and skips scope checks.
	ScopeAll: {
		Role: Role{
			Name:        fmt.Sprintf("Scope_%s", ScopeAll),
			DisplayName: "All operations",
			Site: permissions(map[string][]Action{
				ResourceWildcard.Type: {WildcardSymbol},
			}),
			Org:  map[string][]Permission{},
			User: []Permission{},
		},
		AllowIDList: []string{WildcardSymbol},
	},

	ScopeApplicationConnect: {
		Role: Role{
			Name:        fmt.Sprintf("Scope_%s", ScopeApplicationConnect),
			DisplayName: "Ability to connect to applications",
			Site: permissions(map[string][]Action{
				ResourceWorkspaceApplicationConnect.Type: {ActionCreate},
			}),
			Org:  map[string][]Permission{},
			User: []Permission{},
		},
		AllowIDList: []string{WildcardSymbol},
	},
}

func ExpandScope(scope ScopeName) (Scope, error) {
	role, ok := builtinScopes[scope]
	if !ok {
		return Scope{}, xerrors.Errorf("no scope named %q", scope)
	}
	return role, nil
}
