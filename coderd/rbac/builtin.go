package rbac

import (
	"strings"

	"github.com/google/uuid"

	"golang.org/x/xerrors"
)

const (
	admin   string = "admin"
	member  string = "member"
	auditor string = "auditor"

	orgAdmin  string = "organization-admin"
	orgMember string = "organization-member"
)

// The functions below ONLY need to exist for roles that are "defaulted" in some way.
// Any other roles (like auditor), can be listed and let the user select/assigned.
// Once we have a database implementation, the "default" roles can be defined on the
// site and orgs, and these functions can be removed.

func RoleAdmin() string {
	return roleName(admin, "")
}

func RoleMember() string {
	return roleName(member, "")
}

func RoleOrgAdmin(organizationID uuid.UUID) string {
	return roleName(orgAdmin, organizationID.String())
}

func RoleOrgMember(organizationID uuid.UUID) string {
	return roleName(orgMember, organizationID.String())
}

var (
	// builtInRoles are just a hard coded set for now. Ideally we store these in
	// the database. Right now they are functions because the org id should scope
	// certain roles. When we store them in the database, each organization should
	// create the roles that are assignable in the org. This isn't a hard problem to solve,
	// it's just easier as a function right now.
	//
	// This map will be replaced by database storage defined by this ticket.
	// https://github.com/coder/coder/issues/1194
	builtInRoles = map[string]func(orgID string) Role{
		// admin grants all actions to all resources.
		admin: func(_ string) Role {
			return Role{
				Name:        admin,
				DisplayName: "Admin",
				Site: permissions(map[Object][]Action{
					ResourceWildcard: {WildcardSymbol},
				}),
			}
		},

		// member grants all actions to all resources owned by the user
		member: func(_ string) Role {
			return Role{
				Name:        member,
				DisplayName: "Member",
				Site: permissions(map[Object][]Action{
					// All users can read all other users and know they exist.
					ResourceUser:           {ActionRead},
					ResourceRoleAssignment: {ActionRead},
					// All users can see the provisioner daemons.
					ResourceProvisionerDaemon: {ActionRead},
				}),
				User: permissions(map[Object][]Action{
					ResourceWildcard: {WildcardSymbol},
				}),
			}
		},

		// auditor provides all permissions required to effectively read and understand
		// audit log events.
		// TODO: Finish the auditor as we add resources.
		auditor: func(_ string) Role {
			return Role{
				Name:        "auditor",
				DisplayName: "Auditor",
				Site: permissions(map[Object][]Action{
					// Should be able to read all template details, even in orgs they
					// are not in.
					ResourceTemplate: {ActionRead},
				}),
			}
		},

		// orgAdmin returns a role with all actions allows in a given
		// organization scope.
		orgAdmin: func(organizationID string) Role {
			return Role{
				Name:        roleName(orgAdmin, organizationID),
				DisplayName: "Organization Admin",
				Org: map[string][]Permission{
					organizationID: {
						{
							Negate:       false,
							ResourceType: "*",
							ResourceID:   "*",
							Action:       "*",
						},
					},
				},
			}
		},

		// orgMember has an empty set of permissions, this just implies their membership
		// in an organization.
		orgMember: func(organizationID string) Role {
			return Role{
				Name:        roleName(orgMember, organizationID),
				DisplayName: "Organization Member",
				Org: map[string][]Permission{
					organizationID: {
						{
							// All org members can read the other members in their org.
							ResourceType: ResourceOrganizationMember.Type,
							Action:       ActionRead,
							ResourceID:   "*",
						},
						{
							// All org members can read the organization
							ResourceType: ResourceOrganization.Type,
							Action:       ActionRead,
							ResourceID:   "*",
						},
						{
							// All org members can read templates in the org
							ResourceType: ResourceTemplate.Type,
							Action:       ActionRead,
							ResourceID:   "*",
						},
						{
							// Can read available roles.
							ResourceType: ResourceOrgRoleAssignment.Type,
							ResourceID:   "*",
							Action:       ActionRead,
						},
					},
				},
			}
		},
	}
)

// RoleByName returns the permissions associated with a given role name.
// This allows just the role names to be stored and expanded when required.
func RoleByName(name string) (Role, error) {
	roleName, orgID, err := roleSplit(name)
	if err != nil {
		return Role{}, xerrors.Errorf(":%w", err)
	}

	roleFunc, ok := builtInRoles[roleName]
	if !ok {
		// No role found
		return Role{}, xerrors.Errorf("role %q not found", roleName)
	}

	// Ensure all org roles are properly scoped a non-empty organization id.
	// This is just some defensive programming.
	role := roleFunc(orgID)
	if len(role.Org) > 0 && orgID == "" {
		return Role{}, xerrors.Errorf("expect a org id for role %q", roleName)
	}

	return role, nil
}

func IsOrgRole(roleName string) (string, bool) {
	_, orgID, err := roleSplit(roleName)
	if err == nil && orgID != "" {
		return orgID, true
	}
	return "", false
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
		role := roleF(organizationID.String())
		_, scope, err := roleSplit(role.Name)
		if err != nil {
			// This should never happen
			continue
		}
		if scope == organizationID.String() {
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
		role := roleF("random")
		_, scope, err := roleSplit(role.Name)
		if err != nil {
			// This should never happen
			continue
		}
		if scope == "" {
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
func ChangeRoleSet(from []string, to []string) (added []string, removed []string) {
	has := make(map[string]struct{})
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

// roleName is a quick helper function to return
// 	role_name:scopeID
// If no scopeID is required, only 'role_name' is returned
func roleName(name string, orgID string) string {
	if orgID == "" {
		return name
	}
	return name + ":" + orgID
}

func roleSplit(role string) (name string, orgID string, err error) {
	arr := strings.Split(role, ":")
	if len(arr) > 2 {
		return "", "", xerrors.Errorf("too many colons in role name")
	}

	if arr[0] == "" {
		return "", "", xerrors.Errorf("role cannot be the empty string")
	}

	if len(arr) == 2 {
		return arr[0], arr[1], nil
	}
	return arr[0], "", nil
}

// permissions is just a helper function to make building roles that list out resources
// and actions a bit easier.
func permissions(perms map[Object][]Action) []Permission {
	list := make([]Permission, 0, len(perms))
	for k, actions := range perms {
		for _, act := range actions {
			act := act
			list = append(list, Permission{
				Negate:       false,
				ResourceType: k.Type,
				ResourceID:   WildcardSymbol,
				Action:       act,
			})
		}
	}
	return list
}
