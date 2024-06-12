package rbac

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/ast"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
)

const (
	owner         string = "owner"
	member        string = "member"
	templateAdmin string = "template-admin"
	userAdmin     string = "user-admin"
	auditor       string = "auditor"
	// customSiteRole is a placeholder for all custom site roles.
	// This is used for what roles can assign other roles.
	// TODO: Make this more dynamic to allow other roles to grant.
	customSiteRole string = "custom-site-role"

	orgAdmin  string = "organization-admin"
	orgMember string = "organization-member"
)

func init() {
	// Always load defaults
	ReloadBuiltinRoles(nil)
}

// RoleIdentifiers is a list of user assignable role names. The role names must be
// in the builtInRoles map. Any non-user assignable roles will generate an
// error on Expand.
type RoleIdentifiers []RoleIdentifier

func (names RoleIdentifiers) Expand() ([]Role, error) {
	return rolesByNames(names)
}

func (names RoleIdentifiers) Names() []RoleIdentifier {
	return names
}

// RoleIdentifier contains both the name of the role, and any organizational scope.
// Both fields are required to be globally unique and identifiable.
type RoleIdentifier struct {
	Name string
	// OrganizationID is uuid.Nil for unscoped roles (aka deployment wide)
	OrganizationID uuid.UUID
}

func (r RoleIdentifier) IsOrgRole() bool {
	return r.OrganizationID != uuid.Nil
}

// RoleNameFromString takes a formatted string '<role_name>[:org_id]'.
func RoleNameFromString(input string) (RoleIdentifier, error) {
	var role RoleIdentifier

	arr := strings.Split(input, ":")
	if len(arr) > 2 {
		return role, xerrors.Errorf("too many colons in role name")
	}

	if len(arr) == 0 {
		return role, xerrors.Errorf("empty string not a valid role")
	}

	if arr[0] == "" {
		return role, xerrors.Errorf("role cannot be the empty string")
	}

	role.Name = arr[0]

	if len(arr) == 2 {
		orgID, err := uuid.Parse(arr[1])
		if err != nil {
			return role, xerrors.Errorf("%q not a valid uuid: %w", arr[1], err)
		}
		role.OrganizationID = orgID
	}
	return role, nil
}

func (r RoleIdentifier) String() string {
	if r.OrganizationID != uuid.Nil {
		return r.Name + ":" + r.OrganizationID.String()
	}
	return r.Name
}

func (r RoleIdentifier) UniqueName() string {
	return r.String()
}

func (r *RoleIdentifier) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

func (r *RoleIdentifier) UnmarshalJSON(data []byte) error {
	var str string
	err := json.Unmarshal(data, &str)
	if err != nil {
		return err
	}

	v, err := RoleNameFromString(str)
	if err != nil {
		return err
	}

	*r = v
	return nil
}

// The functions below ONLY need to exist for roles that are "defaulted" in some way.
// Any other roles (like auditor), can be listed and let the user select/assigned.
// Once we have a database implementation, the "default" roles can be defined on the
// site and orgs, and these functions can be removed.

func RoleOwner() RoleIdentifier         { return RoleIdentifier{Name: owner} }
func CustomSiteRole() RoleIdentifier    { return RoleIdentifier{Name: customSiteRole} }
func RoleTemplateAdmin() RoleIdentifier { return RoleIdentifier{Name: templateAdmin} }
func RoleUserAdmin() RoleIdentifier     { return RoleIdentifier{Name: userAdmin} }
func RoleMember() RoleIdentifier        { return RoleIdentifier{Name: member} }
func RoleAuditor() RoleIdentifier       { return RoleIdentifier{Name: auditor} }

func RoleOrgAdmin() string {
	return orgAdmin
}

func RoleOrgMember() string {
	return orgMember
}

// ScopedRoleOrgAdmin is the org role with the organization ID
// Deprecated This was used before organization scope was included as a
// field in all user facing APIs. Usage of 'ScopedRoleOrgAdmin()' is preferred.
func ScopedRoleOrgAdmin(organizationID uuid.UUID) RoleIdentifier {
	return RoleIdentifier{Name: orgAdmin, OrganizationID: organizationID}
}

// ScopedRoleOrgMember is the org role with the organization ID
// Deprecated This was used before organization scope was included as a
// field in all user facing APIs. Usage of 'ScopedRoleOrgMember()' is preferred.
func ScopedRoleOrgMember(organizationID uuid.UUID) RoleIdentifier {
	return RoleIdentifier{Name: orgMember, OrganizationID: organizationID}
}

func allPermsExcept(excepts ...Objecter) []Permission {
	resources := AllResources()
	var perms []Permission
	skip := make(map[string]bool)
	for _, e := range excepts {
		skip[e.RBACObject().Type] = true
	}

	for _, r := range resources {
		// Exceptions
		if skip[r.RBACObject().Type] {
			continue
		}
		// This should always be skipped.
		if r.RBACObject().Type == ResourceWildcard.Type {
			continue
		}
		// Owners can do everything else
		perms = append(perms, Permission{
			Negate:       false,
			ResourceType: r.RBACObject().Type,
			Action:       policy.WildcardSymbol,
		})
	}
	return perms
}

// builtInRoles are just a hard coded set for now. Ideally we store these in
// the database. Right now they are functions because the org id should scope
// certain roles. When we store them in the database, each organization should
// create the roles that are assignable in the org. This isn't a hard problem to solve,
// it's just easier as a function right now.
//
// This map will be replaced by database storage defined by this ticket.
// https://github.com/coder/coder/issues/1194
var builtInRoles map[string]func(orgID uuid.UUID) Role

type RoleOptions struct {
	NoOwnerWorkspaceExec bool
}

// ReloadBuiltinRoles loads the static roles into the builtInRoles map.
// This can be called again with a different config to change the behavior.
//
// TODO: @emyrk This would be great if it was instanced to a coderd rather
// than a global. But that is a much larger refactor right now.
// Essentially we did not foresee different deployments needing slightly
// different role permissions.
func ReloadBuiltinRoles(opts *RoleOptions) {
	if opts == nil {
		opts = &RoleOptions{}
	}

	ownerWorkspaceActions := ResourceWorkspace.AvailableActions()
	if opts.NoOwnerWorkspaceExec {
		// Remove ssh and application connect from the owner role. This
		// prevents owners from have exec access to all workspaces.
		ownerWorkspaceActions = slice.Omit(ownerWorkspaceActions,
			policy.ActionApplicationConnect, policy.ActionSSH)
	}

	// Static roles that never change should be allocated in a closure.
	// This is to ensure these data structures are only allocated once and not
	// on every authorize call. 'withCachedRegoValue' can be used as well to
	// preallocate the rego value that is used by the rego eval engine.
	ownerRole := Role{
		Identifier:  RoleOwner(),
		DisplayName: "Owner",
		Site: append(
			// Workspace dormancy and workspace are omitted.
			// Workspace is specifically handled based on the opts.NoOwnerWorkspaceExec
			allPermsExcept(ResourceWorkspaceDormant, ResourceWorkspace),
			// This adds back in the Workspace permissions.
			Permissions(map[string][]policy.Action{
				ResourceWorkspace.Type:        ownerWorkspaceActions,
				ResourceWorkspaceDormant.Type: {policy.ActionRead, policy.ActionDelete, policy.ActionCreate, policy.ActionUpdate, policy.ActionWorkspaceStop},
			})...),
		Org:  map[string][]Permission{},
		User: []Permission{},
	}.withCachedRegoValue()

	memberRole := Role{
		Identifier:  RoleMember(),
		DisplayName: "Member",
		Site: Permissions(map[string][]policy.Action{
			ResourceAssignRole.Type: {policy.ActionRead},
			// All users can see the provisioner daemons.
			ResourceProvisionerDaemon.Type: {policy.ActionRead},
			// All users can see OAuth2 provider applications.
			ResourceOauth2App.Type:      {policy.ActionRead},
			ResourceWorkspaceProxy.Type: {policy.ActionRead},
		}),
		Org: map[string][]Permission{},
		User: append(allPermsExcept(ResourceWorkspaceDormant, ResourceUser, ResourceOrganizationMember),
			Permissions(map[string][]policy.Action{
				// Reduced permission set on dormant workspaces. No build, ssh, or exec
				ResourceWorkspaceDormant.Type: {policy.ActionRead, policy.ActionDelete, policy.ActionCreate, policy.ActionUpdate, policy.ActionWorkspaceStop},

				// Users cannot do create/update/delete on themselves, but they
				// can read their own details.
				ResourceUser.Type: {policy.ActionRead, policy.ActionReadPersonal, policy.ActionUpdatePersonal},
				// Users can create provisioner daemons scoped to themselves.
				ResourceProvisionerDaemon.Type: {policy.ActionRead, policy.ActionCreate, policy.ActionRead, policy.ActionUpdate},
			})...,
		),
	}.withCachedRegoValue()

	auditorRole := Role{
		Identifier:  RoleAuditor(),
		DisplayName: "Auditor",
		Site: Permissions(map[string][]policy.Action{
			// Should be able to read all template details, even in orgs they
			// are not in.
			ResourceTemplate.Type: {policy.ActionRead, policy.ActionViewInsights},
			ResourceAuditLog.Type: {policy.ActionRead},
			ResourceUser.Type:     {policy.ActionRead},
			ResourceGroup.Type:    {policy.ActionRead},
			// Allow auditors to query deployment stats and insights.
			ResourceDeploymentStats.Type:  {policy.ActionRead},
			ResourceDeploymentConfig.Type: {policy.ActionRead},
			// Org roles are not really used yet, so grant the perm at the site level.
			ResourceOrganizationMember.Type: {policy.ActionRead},
		}),
		Org:  map[string][]Permission{},
		User: []Permission{},
	}.withCachedRegoValue()

	templateAdminRole := Role{
		Identifier:  RoleTemplateAdmin(),
		DisplayName: "Template Admin",
		Site: Permissions(map[string][]policy.Action{
			ResourceTemplate.Type: {policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete, policy.ActionViewInsights},
			// CRUD all files, even those they did not upload.
			ResourceFile.Type:      {policy.ActionCreate, policy.ActionRead},
			ResourceWorkspace.Type: {policy.ActionRead},
			// CRUD to provisioner daemons for now.
			ResourceProvisionerDaemon.Type: {policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete},
			// Needs to read all organizations since
			ResourceOrganization.Type: {policy.ActionRead},
			ResourceUser.Type:         {policy.ActionRead},
			ResourceGroup.Type:        {policy.ActionRead},
			// Org roles are not really used yet, so grant the perm at the site level.
			ResourceOrganizationMember.Type: {policy.ActionRead},
		}),
		Org:  map[string][]Permission{},
		User: []Permission{},
	}.withCachedRegoValue()

	userAdminRole := Role{
		Identifier:  RoleUserAdmin(),
		DisplayName: "User Admin",
		Site: Permissions(map[string][]policy.Action{
			ResourceAssignRole.Type: {policy.ActionAssign, policy.ActionDelete, policy.ActionRead},
			ResourceUser.Type: {
				policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete,
				policy.ActionUpdatePersonal, policy.ActionReadPersonal,
			},
			// Full perms to manage org members
			ResourceOrganizationMember.Type: {policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete},
			ResourceGroup.Type:              {policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete},
		}),
		Org:  map[string][]Permission{},
		User: []Permission{},
	}.withCachedRegoValue()

	builtInRoles = map[string]func(orgID uuid.UUID) Role{
		// admin grants all actions to all resources.
		owner: func(_ uuid.UUID) Role {
			return ownerRole
		},

		// member grants all actions to all resources owned by the user
		member: func(_ uuid.UUID) Role {
			return memberRole
		},

		// auditor provides all permissions required to effectively read and understand
		// audit log events.
		// TODO: Finish the auditor as we add resources.
		auditor: func(_ uuid.UUID) Role {
			return auditorRole
		},

		templateAdmin: func(_ uuid.UUID) Role {
			return templateAdminRole
		},

		userAdmin: func(_ uuid.UUID) Role {
			return userAdminRole
		},

		// orgAdmin returns a role with all actions allows in a given
		// organization scope.
		orgAdmin: func(organizationID uuid.UUID) Role {
			return Role{
				Identifier:  RoleIdentifier{Name: orgAdmin, OrganizationID: organizationID},
				DisplayName: "Organization Admin",
				Site:        []Permission{},
				Org: map[string][]Permission{
					// Org admins should not have workspace exec perms.
					organizationID.String(): append(allPermsExcept(ResourceWorkspace, ResourceWorkspaceDormant), Permissions(map[string][]policy.Action{
						ResourceWorkspaceDormant.Type: {policy.ActionRead, policy.ActionDelete, policy.ActionCreate, policy.ActionUpdate, policy.ActionWorkspaceStop},
						ResourceWorkspace.Type:        slice.Omit(ResourceWorkspace.AvailableActions(), policy.ActionApplicationConnect, policy.ActionSSH),
					})...),
				},
				User: []Permission{},
			}
		},

		// orgMember has an empty set of permissions, this just implies their membership
		// in an organization.
		orgMember: func(organizationID uuid.UUID) Role {
			return Role{
				Identifier:  RoleIdentifier{Name: orgMember, OrganizationID: organizationID},
				DisplayName: "",
				Site:        []Permission{},
				Org: map[string][]Permission{
					organizationID.String(): {
						{
							// All org members can read the organization
							ResourceType: ResourceOrganization.Type,
							Action:       policy.ActionRead,
						},
						{
							// Can read available roles.
							ResourceType: ResourceAssignOrgRole.Type,
							Action:       policy.ActionRead,
						},
					},
				},
				User: []Permission{
					{
						ResourceType: ResourceOrganizationMember.Type,
						Action:       policy.ActionRead,
					},
				},
			}
		},
	}
}

// assignRoles is a map of roles that can be assigned if a user has a given
// role.
// The first key is the actor role, the second is the roles they can assign.
//
//	map[actor_role][assign_role]<can_assign>
var assignRoles = map[string]map[string]bool{
	"system": {
		owner:          true,
		auditor:        true,
		member:         true,
		orgAdmin:       true,
		orgMember:      true,
		templateAdmin:  true,
		userAdmin:      true,
		customSiteRole: true,
	},
	owner: {
		owner:          true,
		auditor:        true,
		member:         true,
		orgAdmin:       true,
		orgMember:      true,
		templateAdmin:  true,
		userAdmin:      true,
		customSiteRole: true,
	},
	userAdmin: {
		member:    true,
		orgMember: true,
	},
	orgAdmin: {
		orgAdmin:  true,
		orgMember: true,
	},
}

// ExpandableRoles is any type that can be expanded into a []Role. This is implemented
// as an interface so we can have RoleIdentifiers for user defined roles, and implement
// custom ExpandableRoles for system type users (eg autostart/autostop system role).
// We want a clear divide between the two types of roles so users have no codepath
// to interact or assign system roles.
//
// Note: We may also want to do the same thing with scopes to allow custom scope
// support unavailable to the user. Eg: Scope to a single resource.
type ExpandableRoles interface {
	Expand() ([]Role, error)
	// Names is for logging and tracing purposes, we want to know the human
	// names of the expanded roles.
	Names() []RoleIdentifier
}

// Permission is the format passed into the rego.
type Permission struct {
	// Negate makes this a negative permission
	Negate       bool          `json:"negate"`
	ResourceType string        `json:"resource_type"`
	Action       policy.Action `json:"action"`
}

func (perm Permission) Valid() error {
	if perm.ResourceType == policy.WildcardSymbol {
		// Wildcard is tricky to check. Just allow it.
		return nil
	}

	resource, ok := policy.RBACPermissions[perm.ResourceType]
	if !ok {
		return xerrors.Errorf("invalid resource type %q", perm.ResourceType)
	}

	// Wildcard action is always valid
	if perm.Action == policy.WildcardSymbol {
		return nil
	}

	_, ok = resource.Actions[perm.Action]
	if !ok {
		return xerrors.Errorf("invalid action %q for resource %q", perm.Action, perm.ResourceType)
	}

	return nil
}

// Role is a set of permissions at multiple levels:
// - Site level permissions apply EVERYWHERE
// - Org level permissions apply to EVERYTHING in a given ORG
// - User level permissions are the lowest
// This is the type passed into the rego as a json payload.
// Users of this package should instead **only** use the role names, and
// this package will expand the role names into their json payloads.
type Role struct {
	Identifier RoleIdentifier `json:"name"`
	// DisplayName is used for UI purposes. If the role has no display name,
	// that means the UI should never display it.
	DisplayName string       `json:"display_name"`
	Site        []Permission `json:"site"`
	// Org is a map of orgid to permissions. We represent orgid as a string.
	// We scope the organizations in the role so we can easily combine all the
	// roles.
	Org  map[string][]Permission `json:"org"`
	User []Permission            `json:"user"`

	// cachedRegoValue can be used to cache the rego value for this role.
	// This is helpful for static roles that never change.
	cachedRegoValue ast.Value
}

// Valid will check all it's permissions and ensure they are all correct
// according to the policy. This verifies every action specified make sense
// for the given resource.
func (role Role) Valid() error {
	var errs []error
	for _, perm := range role.Site {
		if err := perm.Valid(); err != nil {
			errs = append(errs, xerrors.Errorf("site: %w", err))
		}
	}

	for orgID, permissions := range role.Org {
		for _, perm := range permissions {
			if err := perm.Valid(); err != nil {
				errs = append(errs, xerrors.Errorf("org=%q: %w", orgID, err))
			}
		}
	}

	for _, perm := range role.User {
		if err := perm.Valid(); err != nil {
			errs = append(errs, xerrors.Errorf("user: %w", err))
		}
	}

	return errors.Join(errs...)
}

type Roles []Role

func (roles Roles) Expand() ([]Role, error) {
	return roles, nil
}

func (roles Roles) Names() []RoleIdentifier {
	names := make([]RoleIdentifier, 0, len(roles))
	for _, r := range roles {
		names = append(names, r.Identifier)
	}
	return names
}

// CanAssignRole is a helper function that returns true if the user can assign
// the specified role. This also can be used for removing a role.
// This is a simple implementation for now.
func CanAssignRole(subjectHasRoles ExpandableRoles, assignedRole RoleIdentifier) bool {
	// For CanAssignRole, we only care about the names of the roles.
	roles := subjectHasRoles.Names()

	for _, myRole := range roles {
		if myRole.OrganizationID != uuid.Nil && myRole.OrganizationID != assignedRole.OrganizationID {
			// Org roles only apply to the org they are assigned to.
			continue
		}

		allowedAssignList, ok := assignRoles[myRole.Name]
		if !ok {
			continue
		}

		if allowedAssignList[assignedRole.Name] {
			return true
		}
	}
	return false
}

// RoleByName returns the permissions associated with a given role name.
// This allows just the role names to be stored and expanded when required.
//
// This function is exported so that the Display name can be returned to the
// api. We should maybe make an exported function that returns just the
// human-readable content of the Role struct (name + display name).
func RoleByName(name RoleIdentifier) (Role, error) {
	roleFunc, ok := builtInRoles[name.Name]
	if !ok {
		// No role found
		return Role{}, xerrors.Errorf("role %q not found", name.String())
	}

	// Ensure all org roles are properly scoped a non-empty organization id.
	// This is just some defensive programming.
	role := roleFunc(name.OrganizationID)
	if len(role.Org) > 0 && name.OrganizationID == uuid.Nil {
		return Role{}, xerrors.Errorf("expect a org id for role %q", name.String())
	}

	return role, nil
}

func rolesByNames(roleNames []RoleIdentifier) ([]Role, error) {
	roles := make([]Role, 0, len(roleNames))
	for _, n := range roleNames {
		r, err := RoleByName(n)
		if err != nil {
			return nil, xerrors.Errorf("get role permissions: %w", err)
		}
		roles = append(roles, r)
	}
	return roles, nil
}

// OrganizationRoles lists all roles that can be applied to an organization user
// in the given organization. This is the list of available roles,
// and specific to an organization.
//
// This should be a list in a database, but until then we build
// the list from the builtins.
func OrganizationRoles(organizationID uuid.UUID) []Role {
	var roles []Role
	for _, roleF := range builtInRoles {
		role := roleF(organizationID)
		if role.Identifier.OrganizationID == organizationID {
			roles = append(roles, role)
		}
	}
	return roles
}

// SiteRoles lists all roles that can be applied to a user.
// This is the list of available roles, and not specific to a user
//
// This should be a list in a database, but until then we build
// the list from the builtins.
func SiteRoles() []Role {
	var roles []Role
	for _, roleF := range builtInRoles {
		// Must provide some non-nil uuid to filter out org roles.
		role := roleF(uuid.New())
		if !role.Identifier.IsOrgRole() {
			roles = append(roles, role)
		}
	}
	return roles
}

// ChangeRoleSet is a helper function that finds the difference of 2 sets of
// roles. When setting a user's new roles, it is equivalent to adding and
// removing roles. This set determines the changes, so that the appropriate
// RBAC checks can be applied using "ActionCreate" and "ActionDelete" for
// "added" and "removed" roles respectively.
func ChangeRoleSet(from []RoleIdentifier, to []RoleIdentifier) (added []RoleIdentifier, removed []RoleIdentifier) {
	has := make(map[RoleIdentifier]struct{})
	for _, exists := range from {
		has[exists] = struct{}{}
	}

	for _, roleName := range to {
		// If the user already has the role assigned, we don't need to check the permission
		// to reassign it. Only run permission checks on the difference in the set of
		// roles.
		if _, ok := has[roleName]; ok {
			delete(has, roleName)
			continue
		}

		added = append(added, roleName)
	}

	// Remaining roles are the ones removed/deleted.
	for roleName := range has {
		removed = append(removed, roleName)
	}

	return added, removed
}

// Permissions is just a helper function to make building roles that list out resources
// and actions a bit easier.
func Permissions(perms map[string][]policy.Action) []Permission {
	list := make([]Permission, 0, len(perms))
	for k, actions := range perms {
		for _, act := range actions {
			act := act
			list = append(list, Permission{
				Negate:       false,
				ResourceType: k,
				Action:       act,
			})
		}
	}
	// Deterministic ordering of permissions
	sort.Slice(list, func(i, j int) bool {
		return list[i].ResourceType < list[j].ResourceType
	})
	return list
}
