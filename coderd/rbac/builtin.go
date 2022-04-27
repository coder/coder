package rbac

import (
	"github.com/google/uuid"
	"strings"

	"golang.org/x/xerrors"
)

const (
	admin   string = "admin"
	member  string = "member"
	auditor string = "auditor"

	orgAdmin  string = "organization-admin"
	orgMember string = "organization-member"
)

// RoleName is a string that represents a registered rbac role. We want to store
// strings in the database to allow the underlying role permissions to be migrated
// or modified.
// All RoleNames should have an entry in 'builtInRoles'.
// We use functions to retrieve the name incase we need to add a scope.
type RoleName = string

// The functions below ONLY need to exist for roles that are "defaulted" in some way.
// Any other roles (like auditor), can be listed and let the user select/assigned.
// Once we have a database implementation, the "default" roles can be defined on the
// site and orgs, and these functions can be removed.

func RoleAdmin() RoleName {
	return roleName(admin, "")
}

func RoleMember() RoleName {
	return roleName(member, "")
}

func RoleOrgAdmin(organizationID uuid.UUID) RoleName {
	return roleName(orgAdmin, organizationID.String())
}

// member:uuid
func RoleOrgMember(organizationID uuid.UUID) RoleName {
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
	builtInRoles = map[string]func(scopeID string) Role{
		// admin grants all actions to all resources.
		admin: func(_ string) Role {
			return Role{
				Name: admin,
				Site: permissions(map[Object][]Action{
					ResourceWildcard: {WildcardSymbol},
				}),
			}
		},

		// member grants all actions to all resources owned by the user
		member: func(_ string) Role {
			return Role{
				Name: member,
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
				Name: "auditor",
				Site: permissions(map[Object][]Action{
					//ResourceAuditLogs: {ActionRead},
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
				Name: roleName(orgAdmin, organizationID),
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
				Name: roleName(orgMember, organizationID),
				Org: map[string][]Permission{
					organizationID: {},
				},
			}
		},
	}
)

// ListOrgRoles lists all roles that can be applied to an organization user
// in the given organization.
// Note: This should be a list in a database, but until then we build
// 	the list from the builtins.
func ListOrgRoles(organizationID uuid.UUID) []string {
	var roles []string
	for role, _ := range builtInRoles {
		_, scope, err := roleSplit(role)
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

// ListSiteRoles lists all roles that can be applied to a user.
// Note: This should be a list in a database, but until then we build
// 	the list from the builtins.
func ListSiteRoles() []string {
	var roles []string
	for role, _ := range builtInRoles {
		_, scope, err := roleSplit(role)
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

// RoleByName returns the permissions associated with a given role name.
// This allows just the role names to be stored and expanded when required.
func RoleByName(name string) (Role, error) {
	roleName, scopeID, err := roleSplit(name)
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
	role := roleFunc(scopeID)
	if len(role.Org) > 0 && scopeID == "" {
		return Role{}, xerrors.Errorf("expect a scope id for role %q", roleName)
	}

	return role, nil
}

// roleName is a quick helper function to return
// 	role_name:scopeID
// If no scopeID is required, only 'role_name' is returned
func roleName(name string, scopeID string) string {
	if scopeID == "" {
		return name
	}
	return name + ":" + scopeID
}

func roleSplit(role string) (name string, scopeID string, err error) {
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

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
