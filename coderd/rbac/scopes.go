package rbac

import "golang.org/x/xerrors"

const (
	ScopeAny      = "any"
	ScopeReadonly = "readonly"
)

var builtinScopes map[string]Role = map[string]Role{
	ScopeAny: {
		Name:        ScopeAny,
		DisplayName: "Any operation",
		Site: permissions(map[Object][]Action{
			ResourceWildcard: {WildcardSymbol},
		}),
		Org:  map[string][]Permission{},
		User: []Permission{},
	},

	ScopeReadonly: {
		Name:        ScopeReadonly,
		DisplayName: "Only read operations",
		Site: permissions(map[Object][]Action{
			ResourceWildcard: {ActionRead},
		}),
		Org:  map[string][]Permission{},
		User: []Permission{},
	},
}

func ScopeByName(name string) (Role, error) {
	scope, ok := builtinScopes[name]
	if !ok {
		return Role{}, xerrors.Errorf("no role named %q", name)
	}
	return scope, nil
}
