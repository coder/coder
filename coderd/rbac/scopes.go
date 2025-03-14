package rbac
import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)
type WorkspaceAgentScopeParams struct {
	WorkspaceID uuid.UUID
	OwnerID     uuid.UUID
	TemplateID  uuid.UUID
	VersionID   uuid.UUID
}
// WorkspaceAgentScope returns a scope that is the same as ScopeAll but can only
// affect resources in the allow list. Only a scope is returned as the roles
// should come from the workspace owner.
func WorkspaceAgentScope(params WorkspaceAgentScopeParams) Scope {
	if params.WorkspaceID == uuid.Nil || params.OwnerID == uuid.Nil || params.TemplateID == uuid.Nil || params.VersionID == uuid.Nil {
		panic("all uuids must be non-nil, this is a developer error")
	}
	allScope, err := ScopeAll.Expand()
	if err != nil {
		panic("failed to expand scope all, this should never happen")
	}
	return Scope{
		// TODO: We want to limit the role too to be extra safe.
		// Even though the allowlist blocks anything else, it is still good
		// incase we change the behavior of the allowlist. The allowlist is new
		// and evolving.
		Role: allScope.Role,
		// This prevents the agent from being able to access any other resource.
		// Include the list of IDs of anything that is required for the
		// agent to function.
		AllowIDList: []string{
			params.WorkspaceID.String(),
			params.TemplateID.String(),
			params.VersionID.String(),
			params.OwnerID.String(),
		},
	}
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
			Identifier:  RoleIdentifier{Name: fmt.Sprintf("Scope_%s", ScopeAll)},
			DisplayName: "All operations",
			Site: Permissions(map[string][]policy.Action{
				ResourceWildcard.Type: {policy.WildcardSymbol},
			}),
			Org:  map[string][]Permission{},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},
	ScopeApplicationConnect: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: fmt.Sprintf("Scope_%s", ScopeApplicationConnect)},
			DisplayName: "Ability to connect to applications",
			Site: Permissions(map[string][]policy.Action{
				ResourceWorkspace.Type: {policy.ActionApplicationConnect},
			}),
			Org:  map[string][]Permission{},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},
}
type ExpandableScope interface {
	Expand() (Scope, error)
	// Name is for logging and tracing purposes, we want to know the human
	// name of the scope.
	Name() RoleIdentifier
}
type ScopeName string
func (name ScopeName) Expand() (Scope, error) {
	return ExpandScope(name)
}
func (name ScopeName) Name() RoleIdentifier {
	return RoleIdentifier{Name: string(name)}
}
// Scope acts the exact same as a Role with the addition that is can also
// apply an AllowIDList. Any resource being checked against a Scope will
// reject any resource that is not in the AllowIDList.
// To not use an AllowIDList to reject authorization, use a wildcard for the
// AllowIDList. Eg: 'AllowIDList: []string{WildcardSymbol}'
type Scope struct {
	Role
	AllowIDList []string `json:"allow_list"`
}
func (s Scope) Expand() (Scope, error) {
	return s, nil
}
func (s Scope) Name() RoleIdentifier {
	return s.Role.Identifier
}
func ExpandScope(scope ScopeName) (Scope, error) {
	role, ok := builtinScopes[scope]
	if !ok {
		return Scope{}, fmt.Errorf("no scope named %q", scope)
	}
	return role, nil
}
