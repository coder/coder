package rbac

import (
	"fmt"

	"golang.org/x/xerrors"
)

type ScopeName string

// TODO: @emyrk rename this struct
type ScopeRole struct {
	Role
	AllowIDList []string `json:"allow_list"`
}

const (
	ScopeAll                ScopeName = "all"
	ScopeApplicationConnect ScopeName = "application_connect"
)

// TODO: Support passing in scopeID list for allowlisting resources.
var builtinScopes = map[ScopeName]ScopeRole{
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

func ExpandScope(scope ScopeName) (ScopeRole, error) {
	role, ok := builtinScopes[scope]
	if !ok {
		return ScopeRole{}, xerrors.Errorf("no scope named %q", scope)
	}
	return role, nil
}
