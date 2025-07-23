package rbac

import (
	"fmt"

	"github.com/google/uuid"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

type WorkspaceAgentScopeParams struct {
	WorkspaceID   uuid.UUID
	OwnerID       uuid.UUID
	TemplateID    uuid.UUID
	VersionID     uuid.UUID
	BlockUserData bool
}

// WorkspaceAgentScope returns a scope that is the same as ScopeAll but can only
// affect resources in the allow list. Only a scope is returned as the roles
// should come from the workspace owner.
func WorkspaceAgentScope(params WorkspaceAgentScopeParams) Scope {
	if params.WorkspaceID == uuid.Nil || params.OwnerID == uuid.Nil || params.TemplateID == uuid.Nil || params.VersionID == uuid.Nil {
		panic("all uuids must be non-nil, this is a developer error")
	}

	var (
		scope Scope
		err   error
	)
	if params.BlockUserData {
		scope, err = ScopeNoUserData.Expand()
	} else {
		scope, err = ScopeAll.Expand()
	}
	if err != nil {
		panic("failed to expand scope, this should never happen")
	}

	return Scope{
		// TODO: We want to limit the role too to be extra safe.
		// Even though the allowlist blocks anything else, it is still good
		// incase we change the behavior of the allowlist. The allowlist is new
		// and evolving.
		Role: scope.Role,
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
	// Existing scopes (unchanged)
	ScopeAll                ScopeName = "all"
	ScopeApplicationConnect ScopeName = "application_connect"
	ScopeNoUserData         ScopeName = "no_user_data"

	// New granular scopes
	ScopeUserRead          ScopeName = "user:read"
	ScopeUserWrite         ScopeName = "user:write"
	ScopeWorkspaceRead     ScopeName = "workspace:read"
	ScopeWorkspaceWrite    ScopeName = "workspace:write"
	ScopeWorkspaceSSH      ScopeName = "workspace:ssh"
	ScopeWorkspaceApps     ScopeName = "workspace:apps"
	ScopeTemplateRead      ScopeName = "template:read"
	ScopeTemplateWrite     ScopeName = "template:write"
	ScopeOrganizationRead  ScopeName = "organization:read"
	ScopeOrganizationWrite ScopeName = "organization:write"
	ScopeAuditRead         ScopeName = "audit:read"
	ScopeSystemRead        ScopeName = "system:read"
	ScopeSystemWrite       ScopeName = "system:write"
)

// AdditionalPermissions represents additional permissions for write scopes
type AdditionalPermissions struct {
	Site map[string][]policy.Action // Site-level permissions
	Org  map[string][]Permission    // Organization-level permissions
	User []Permission               // User-level permissions
}

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

	ScopeNoUserData: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: fmt.Sprintf("Scope_%s", ScopeNoUserData)},
			DisplayName: "Scope without access to user data",
			Site:        allPermsExcept(ResourceUser),
			Org:         map[string][]Permission{},
			User:        []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	// User scopes (read + write pair)
	ScopeUserRead: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_user:read"},
			DisplayName: "Read user profile",
			Site: Permissions(map[string][]policy.Action{
				ResourceUser.Type: {policy.ActionReadPersonal},
			}),
			Org:  map[string][]Permission{},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	ScopeUserWrite: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_user:write"},
			DisplayName: "Manage user profile",
			Site: Permissions(map[string][]policy.Action{
				ResourceUser.Type: {policy.ActionReadPersonal, policy.ActionUpdatePersonal},
			}),
			Org:  map[string][]Permission{},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	// Workspace scopes (read + write pair)
	ScopeWorkspaceRead: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_workspace:read"},
			DisplayName: "Read workspaces",
			Site: Permissions(map[string][]policy.Action{
				ResourceWorkspace.Type: {policy.ActionRead},
			}),
			Org: map[string][]Permission{
				ResourceWorkspace.Type: {{ResourceType: ResourceWorkspace.Type, Action: policy.ActionRead}},
			},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	ScopeWorkspaceWrite: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_workspace:write"},
			DisplayName: "Manage workspaces",
			Site: Permissions(map[string][]policy.Action{
				ResourceWorkspace.Type: {policy.ActionRead, policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			}),
			Org: map[string][]Permission{
				ResourceWorkspace.Type: {
					{ResourceType: ResourceWorkspace.Type, Action: policy.ActionRead},
					{ResourceType: ResourceWorkspace.Type, Action: policy.ActionCreate},
					{ResourceType: ResourceWorkspace.Type, Action: policy.ActionUpdate},
					{ResourceType: ResourceWorkspace.Type, Action: policy.ActionDelete},
				},
			},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	// Workspace special scopes (SSH and Apps)
	ScopeWorkspaceSSH: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_workspace:ssh"},
			DisplayName: "SSH to workspaces",
			Site: Permissions(map[string][]policy.Action{
				ResourceWorkspace.Type: {policy.ActionSSH},
			}),
			Org: map[string][]Permission{
				ResourceWorkspace.Type: {{ResourceType: ResourceWorkspace.Type, Action: policy.ActionSSH}},
			},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	ScopeWorkspaceApps: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_workspace:apps"},
			DisplayName: "Connect to workspace applications",
			Site: Permissions(map[string][]policy.Action{
				ResourceWorkspace.Type: {policy.ActionApplicationConnect},
			}),
			Org: map[string][]Permission{
				ResourceWorkspace.Type: {{ResourceType: ResourceWorkspace.Type, Action: policy.ActionApplicationConnect}},
			},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	// Template scopes (read + write pair)
	ScopeTemplateRead: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_template:read"},
			DisplayName: "Read templates",
			Site: Permissions(map[string][]policy.Action{
				ResourceTemplate.Type: {policy.ActionRead},
			}),
			Org: map[string][]Permission{
				ResourceTemplate.Type: {{ResourceType: ResourceTemplate.Type, Action: policy.ActionRead}},
			},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	ScopeTemplateWrite: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_template:write"},
			DisplayName: "Manage templates",
			Site: Permissions(map[string][]policy.Action{
				ResourceTemplate.Type: {policy.ActionRead, policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			}),
			Org: map[string][]Permission{
				ResourceTemplate.Type: {
					{ResourceType: ResourceTemplate.Type, Action: policy.ActionRead},
					{ResourceType: ResourceTemplate.Type, Action: policy.ActionCreate},
					{ResourceType: ResourceTemplate.Type, Action: policy.ActionUpdate},
					{ResourceType: ResourceTemplate.Type, Action: policy.ActionDelete},
				},
			},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	// Organization scopes (read + write pair)
	ScopeOrganizationRead: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_organization:read"},
			DisplayName: "Read organization",
			Site: Permissions(map[string][]policy.Action{
				ResourceOrganization.Type: {policy.ActionRead},
			}),
			Org: map[string][]Permission{
				ResourceOrganization.Type: {{ResourceType: ResourceOrganization.Type, Action: policy.ActionRead}},
			},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	ScopeOrganizationWrite: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_organization:write"},
			DisplayName: "Manage organization",
			Site: Permissions(map[string][]policy.Action{
				ResourceOrganization.Type: {policy.ActionRead, policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			}),
			Org: map[string][]Permission{
				ResourceOrganization.Type: {
					{ResourceType: ResourceOrganization.Type, Action: policy.ActionRead},
					{ResourceType: ResourceOrganization.Type, Action: policy.ActionCreate},
					{ResourceType: ResourceOrganization.Type, Action: policy.ActionUpdate},
					{ResourceType: ResourceOrganization.Type, Action: policy.ActionDelete},
				},
			},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	// Audit scopes (read only - no write needed)
	ScopeAuditRead: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_audit:read"},
			DisplayName: "Read audit logs",
			Site: Permissions(map[string][]policy.Action{
				ResourceAuditLog.Type: {policy.ActionRead},
			}),
			Org:  map[string][]Permission{},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	// System scopes (read + write pair)
	ScopeSystemRead: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_system:read"},
			DisplayName: "Read system information",
			Site: Permissions(map[string][]policy.Action{
				ResourceSystem.Type: {policy.ActionRead},
			}),
			Org:  map[string][]Permission{},
			User: []Permission{},
		},
		AllowIDList: []string{policy.WildcardSymbol},
	},

	ScopeSystemWrite: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: "Scope_system:write"},
			DisplayName: "Manage system",
			Site: Permissions(map[string][]policy.Action{
				ResourceSystem.Type: {policy.ActionRead, policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
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
		return Scope{}, xerrors.Errorf("no scope named %q", scope)
	}
	return role, nil
}
