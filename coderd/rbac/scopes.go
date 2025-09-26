package rbac

import (
	"fmt"
	"slices"
	"strings"

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

		// Limit the agent to only be able to access the singular workspace and
		// the template/version it was created from. Add additional resources here
		// as needed, but do not add more workspace or template resource ids.
		AllowIDList: []AllowListElement{
			{Type: ResourceWorkspace.Type, ID: params.WorkspaceID.String()},
			{Type: ResourceTemplate.Type, ID: params.TemplateID.String()},
			{Type: ResourceTemplate.Type, ID: params.VersionID.String()},
			{Type: ResourceUser.Type, ID: params.OwnerID.String()},
		},
	}
}

const (
	ScopeAll                ScopeName = "all"
	ScopeApplicationConnect ScopeName = "application_connect"
	ScopeNoUserData         ScopeName = "no_user_data"
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
			Org:       map[string][]Permission{},
			User:      []Permission{},
			OrgMember: map[string][]Permission{},
		},
		AllowIDList: []AllowListElement{AllowListAll()},
	},

	ScopeApplicationConnect: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: fmt.Sprintf("Scope_%s", ScopeApplicationConnect)},
			DisplayName: "Ability to connect to applications",
			Site: Permissions(map[string][]policy.Action{
				ResourceWorkspace.Type: {policy.ActionApplicationConnect},
			}),
			Org:       map[string][]Permission{},
			User:      []Permission{},
			OrgMember: map[string][]Permission{},
		},
		AllowIDList: []AllowListElement{AllowListAll()},
	},

	ScopeNoUserData: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: fmt.Sprintf("Scope_%s", ScopeNoUserData)},
			DisplayName: "Scope without access to user data",
			Site:        allPermsExcept(ResourceUser),
			Org:         map[string][]Permission{},
			User:        []Permission{},
			OrgMember:   map[string][]Permission{},
		},
		AllowIDList: []AllowListElement{AllowListAll()},
	},
}

// BuiltinScopeNames returns the list of built-in high-level scope names
// defined in this package (e.g., "all", "application_connect"). The result
// is sorted for deterministic ordering in code generation and tests.
func BuiltinScopeNames() []ScopeName {
	names := make([]ScopeName, 0, len(builtinScopes))
	for name := range builtinScopes {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
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
	AllowIDList []AllowListElement `json:"allow_list"`
}

type AllowListElement struct {
	// ID must be a string to allow for the wildcard symbol.
	ID   string `json:"id"`
	Type string `json:"type"`
}

func AllowListAll() AllowListElement {
	return AllowListElement{ID: policy.WildcardSymbol, Type: policy.WildcardSymbol}
}

// String encodes the allow list element into the canonical database representation
// "type:id". This avoids fragile manual concatenations scattered across the codebase.
func (e AllowListElement) String() string {
	return e.Type + ":" + e.ID
}

func (s Scope) Expand() (Scope, error) {
	return s, nil
}

func (s Scope) Name() RoleIdentifier {
	return s.Identifier
}

func ExpandScope(scope ScopeName) (Scope, error) {
	if role, ok := builtinScopes[scope]; ok {
		return role, nil
	}
	if res, act, ok := parseLowLevelScope(scope); ok {
		return expandLowLevel(res, act), nil
	}
	return Scope{}, xerrors.Errorf("no scope named %q", scope)
}

// ParseResourceAction parses a scope string formatted as "<resource>:<action>"
// and returns the resource and action components. This is the common parsing
// logic shared between RBAC and database validation.
func ParseResourceAction(scope string) (resource string, action string, ok bool) {
	parts := strings.SplitN(scope, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// parseLowLevelScope parses a low-level scope name formatted as
// "<resource>:<action>" and validates it against RBACPermissions.
// Returns the resource and action if valid.
func parseLowLevelScope(name ScopeName) (resource string, action policy.Action, ok bool) {
	res, act, ok := ParseResourceAction(string(name))
	if !ok {
		return "", "", false
	}

	def, exists := policy.RBACPermissions[res]
	if !exists {
		return "", "", false
	}
	if _, exists := def.Actions[policy.Action(act)]; !exists {
		return "", "", false
	}
	return res, policy.Action(act), true
}

// expandLowLevel constructs a site-only Scope with a single permission for the
// given resource and action. This mirrors how builtin scopes are represented
// but is restricted to site-level only.
func expandLowLevel(resource string, action policy.Action) Scope {
	return Scope{
		Role: Role{
			Identifier:  RoleIdentifier{Name: fmt.Sprintf("Scope_%s:%s", resource, action)},
			DisplayName: fmt.Sprintf("%s:%s", resource, action),
			Site:        []Permission{{ResourceType: resource, Action: action}},
			Org:         map[string][]Permission{},
			User:        []Permission{},
			OrgMember:   map[string][]Permission{},
		},
		// Low-level scopes intentionally return an empty allow list.
		AllowIDList: []AllowListElement{},
	}
}
