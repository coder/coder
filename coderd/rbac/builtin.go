package rbac

import (
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

func RoleAdmin() string {
	return roleName(admin, "")
}

func RoleMember() string {
	return roleName(member, "")
}

func RoleOrgAdmin(organizationID string) RoleName {
	return roleName(orgAdmin, organizationID)
}

func RoleOrgMember(organizationID string) RoleName {
	return roleName(orgMember, organizationID)
}

var (
	// builtInRoles are just a hard coded set for now. Ideally we store these in
	// the database. Right now they are functions because the org id should scope
	// certain roles. If we store them in the database, we will need to store
	// them such that the "org" permissions are dynamically changed by the
	// scopeID passed in. This isn't a hard problem to solve, it's just easier
	// as a function right now.
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

// RoleByName returns the permissions associated with a given role name.
// This allows just the role names to be stored and expanded when required.
func RoleByName(name string) (Role, error) {
	arr := strings.Split(name, ":")
	if len(arr) > 2 {
		return Role{}, xerrors.Errorf("too many semicolons in role name")
	}

	roleName := arr[0]
	var scopeID string
	if len(arr) > 1 {
		scopeID = arr[1]
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
