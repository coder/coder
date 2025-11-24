package rbac

import (
	"fmt"
	"slices"
	"sort"
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
	TaskID        uuid.NullUUID
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

	// Include task in the allow list if the workspace has an associated task.
	var extraAllowList []AllowListElement
	if params.TaskID.Valid {
		extraAllowList = append(extraAllowList, AllowListElement{
			Type: ResourceTask.Type,
			ID:   params.TaskID.UUID.String(),
		})
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
		AllowIDList: append([]AllowListElement{
			{Type: ResourceWorkspace.Type, ID: params.WorkspaceID.String()},
			{Type: ResourceTemplate.Type, ID: params.TemplateID.String()},
			{Type: ResourceTemplate.Type, ID: params.VersionID.String()},
			{Type: ResourceUser.Type, ID: params.OwnerID.String()},
		}, extraAllowList...),
	}
}

const (
	ScopeAll                ScopeName = "coder:all"
	ScopeApplicationConnect ScopeName = "coder:application_connect"
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
			User:    []Permission{},
			ByOrgID: map[string]OrgPermissions{},
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
			User:    []Permission{},
			ByOrgID: map[string]OrgPermissions{},
		},
		AllowIDList: []AllowListElement{AllowListAll()},
	},

	ScopeNoUserData: {
		Role: Role{
			Identifier:  RoleIdentifier{Name: fmt.Sprintf("Scope_%s", ScopeNoUserData)},
			DisplayName: "Scope without access to user data",
			Site:        allPermsExcept(ResourceUser),
			User:        []Permission{},
			ByOrgID:     map[string]OrgPermissions{},
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

// Composite coder:* scopes expand to multiple low-level resource:action permissions
// at Site level. These names are persisted in the DB and expanded during
// authorization.
var compositePerms = map[ScopeName]map[string][]policy.Action{
	"coder:workspaces.create": {
		ResourceTemplate.Type:  {policy.ActionRead, policy.ActionUse},
		ResourceWorkspace.Type: {policy.ActionCreate, policy.ActionUpdate, policy.ActionRead},
	},
	"coder:workspaces.operate": {
		ResourceWorkspace.Type: {policy.ActionRead, policy.ActionUpdate},
	},
	"coder:workspaces.delete": {
		ResourceWorkspace.Type: {policy.ActionRead, policy.ActionDelete},
	},
	"coder:workspaces.access": {
		ResourceWorkspace.Type: {policy.ActionRead, policy.ActionSSH, policy.ActionApplicationConnect},
	},
	"coder:templates.build": {
		ResourceTemplate.Type: {policy.ActionRead},
		ResourceFile.Type:     {policy.ActionCreate, policy.ActionRead},
		"provisioner_jobs":    {policy.ActionRead},
	},
	"coder:templates.author": {
		ResourceTemplate.Type: {policy.ActionRead, policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete, policy.ActionViewInsights},
		ResourceFile.Type:     {policy.ActionCreate, policy.ActionRead},
	},
	"coder:apikeys.manage_self": {
		ResourceApiKey.Type: {policy.ActionRead, policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
	},
}

// CompositeSitePermissions returns the site-level Permission list for a coder:* scope.
func CompositeSitePermissions(name ScopeName) ([]Permission, bool) {
	perms, ok := compositePerms[name]
	if !ok {
		return nil, false
	}
	return Permissions(perms), true
}

// CompositeScopeNames lists all high-level coder:* names in sorted order.
func CompositeScopeNames() []string {
	out := make([]string, 0, len(compositePerms))
	for k := range compositePerms {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
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
	if site, ok := CompositeSitePermissions(scope); ok {
		return Scope{
			Role: Role{
				Identifier:  RoleIdentifier{Name: fmt.Sprintf("Scope_%s", scope)},
				DisplayName: string(scope),
				Site:        site,
				User:        []Permission{},
				ByOrgID:     map[string]OrgPermissions{},
			},
			// Composites are site-level; allow-list empty by default
			AllowIDList: []AllowListElement{{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol}},
		}, nil
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

	if act == policy.WildcardSymbol {
		return res, policy.WildcardSymbol, true
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
			User:        []Permission{},
			ByOrgID:     map[string]OrgPermissions{},
		},
		// Low-level scopes intentionally return a wildcard allow list.
		AllowIDList: []AllowListElement{{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol}},
	}
}
