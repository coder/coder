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
			// No pre-existing ID for new records; wildcard is required.
			// Owner-scoped create (user-level) limits agents to their own
			// logs. Adding site-level actions to the member role would
			// bypass this and grant deployment-wide access.
			{Type: ResourceBoundaryLog.Type, ID: policy.WildcardSymbol},
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
		ResourceWorkspace.Type: {policy.ActionWorkspaceStop, policy.ActionWorkspaceStart, policy.ActionCreate, policy.ActionUpdate, policy.ActionRead},
		// When creating a workspace, users need to be able to read the org member the
		// workspace will be owned by. Even if that owner is "yourself".
		ResourceOrganizationMember.Type: {policy.ActionRead},
	},
	"coder:workspaces.operate": {
		ResourceTemplate.Type:           {policy.ActionRead},
		ResourceWorkspace.Type:          {policy.ActionWorkspaceStop, policy.ActionWorkspaceStart, policy.ActionRead, policy.ActionUpdate},
		ResourceOrganizationMember.Type: {policy.ActionRead},
	},
	"coder:workspaces.delete": {
		ResourceTemplate.Type:           {policy.ActionRead, policy.ActionUse},
		ResourceWorkspace.Type:          {policy.ActionRead, policy.ActionDelete},
		ResourceOrganizationMember.Type: {policy.ActionRead},
	},
	"coder:workspaces.access": {
		ResourceTemplate.Type:           {policy.ActionRead},
		ResourceOrganizationMember.Type: {policy.ActionRead},
		ResourceWorkspace.Type:          {policy.ActionRead, policy.ActionSSH, policy.ActionApplicationConnect},
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
	slices.Sort(out)
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

// ExpandScope resolves a scope name to the permissions it grants, from the
// builtin scopes, the composite coder:* scopes, or a low-level resource:action
// pair. The name must be canonical: the `all` and `application_connect`
// aliases IsExternalScope accepts are not scope names here, so canonicalize
// with CanonicalScopeName first.
//
// Every expansion populates Site only, with a wildcard allow list and no
// negative permissions. Keep new scopes within that shape. ScopesCover depends
// on it, and a scope that breaks it becomes uncomparable: coverage refuses it
// on either side rather than answering from the fraction it does read.
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

// ScopesCover reports whether every permission the requested scope grants is
// also granted by at least one of the allowed scopes. It compares expanded
// permissions, not names, so `coder:workspaces.access` covers `workspace:read`
// and `coder:all` covers everything.
//
// A wildcard request is covered only by a wildcard grant. Enumerating every
// workspace action that exists today does not cover `workspace:*`, because the
// wildcard also authorizes the actions added tomorrow. Rejecting a wildcard
// against an allowlist that looks exhaustive is the intended answer, not a gap
// to close.
//
// Both sides must already be canonical, which the parameter names restate at
// every call site. Passing what IsExternalScope accepted is not enough: it
// admits the `all` and `application_connect` aliases, which are public
// spellings rather than expandable names, so canonicalize between validating a
// name and asking about its coverage. An unknown name is an error rather than
// a false, since a caller cannot tell those two apart.
//
// Coverage models site-level grants only. A scope carrying anything else is
// refused on either side rather than compared on the part that is modeled,
// because comparing a subset could report "covered" about authority that was
// never examined.
func ScopesCover(canonicalAllowed []ScopeName, canonicalRequested ScopeName) (bool, error) {
	want, err := ExpandScope(canonicalRequested)
	if err != nil {
		return false, xerrors.Errorf("expand requested scope: %w", err)
	}

	grants := make([]namedScope, 0, len(canonicalAllowed))
	for _, name := range canonicalAllowed {
		expanded, err := ExpandScope(name)
		if err != nil {
			return false, xerrors.Errorf("expand allowed scope %q: %w", name, err)
		}
		grants = append(grants, namedScope{name: name, scope: expanded})
	}

	return scopesCoverExpanded(grants, namedScope{name: canonicalRequested, scope: want})
}

// namedScope pairs an expanded scope with the name the caller spelled, so a
// guard error can name the scope as it was requested rather than as it expanded.
type namedScope struct {
	name  ScopeName
	scope Scope
}

// scopesCoverExpanded is the comparison ScopesCover runs once both sides are
// expanded. It is separate because every Scope ExpandScope builds satisfies
// the guards below, so driving synthetic Scope values through this function is
// the only way to reach them. Testing checkCoverable alone would leave
// unverified the part that matters most: that both sides are actually checked.
func scopesCoverExpanded(allowed []namedScope, requested namedScope) (bool, error) {
	if err := checkCoverable(requested.scope, coverageSideRequested, requested.name); err != nil {
		return false, err
	}

	granted := make([]Permission, 0, len(allowed)*4)
	for _, entry := range allowed {
		if err := checkCoverable(entry.scope, coverageSideAllowed, entry.name); err != nil {
			return false, err
		}
		granted = append(granted, entry.scope.Site...)
	}

	for _, needed := range requested.scope.Site {
		if !permissionCovered(needed, granted) {
			return false, nil
		}
	}
	return true, nil
}

// Which side of a coverage comparison a scope sits on. Both sides are held to
// the same invariant, so the side only distinguishes the error messages.
const (
	coverageSideRequested = "requested"
	coverageSideAllowed   = "allowed"
)

// checkCoverable reports an error when scope carries authority that coverage
// cannot compare. Scope expansion populates Site only, with a wildcard allow
// list and no negative permissions, and coverage reads nothing else. A scope
// that breaks the invariant is refused rather than compared on its Site
// permissions alone, since the permissions left unread could be the ones that
// decide the answer: an org or user grant may itself carry a negative
// permission, and an "everything except delete" scope must not end up covering
// a request for delete. An allow list makes the Site permissions conditional,
// and reading them as unconditional would overstate the authority granted.
func checkCoverable(scope Scope, side string, name ScopeName) error {
	if len(scope.User) > 0 || len(scope.ByOrgID) > 0 {
		return xerrors.Errorf("%s scope %q grants org or user permissions, which coverage does not model", side, name)
	}
	for _, perm := range scope.Site {
		if perm.Negate {
			return xerrors.Errorf("%s scope %q carries a negative permission, which coverage does not model", side, name)
		}
	}
	if !allowListContainsAll(scope.AllowIDList) {
		return xerrors.Errorf("%s scope %q carries a resource allow list, which coverage does not model", side, name)
	}
	return nil
}

// permissionCovered reports whether any granted permission subsumes needed,
// treating the wildcard resource type and action as covering every value.
//
// granted must carry no negative permissions. checkCoverable refuses a scope
// holding one before ScopesCover gets here, because subsumption is the wrong
// question to ask about an anti-grant. Skipping a negative leaves any wildcard
// beside it free to match, so an "everything except delete" scope would read
// as covering delete, and honoring one as a grant would be worse still.
func permissionCovered(needed Permission, granted []Permission) bool {
	for _, perm := range granted {
		if perm.ResourceType != needed.ResourceType && perm.ResourceType != policy.WildcardSymbol {
			continue
		}
		if perm.Action != needed.Action && perm.Action != policy.WildcardSymbol {
			continue
		}
		return true
	}
	return false
}
