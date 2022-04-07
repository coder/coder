/*
Package rbac provides role based access control primitives.

Roles, Resources, Operations

Roles, resources, and operations are modeled as strings and exposed as constants. Constants are in CamelCaps case. The
strings they represent are in kebab-case. E.g.,

        const SuperAdmin rbac.Role = "super-admin"

Roles

Indicate the scope of a role by beginning the name with said scope. This helps differentiate between a SiteAdmin and
an OrganizationAdmin, for example.

Resources

Resources are limited to types that exist in the application. E.g., Workspaces or Images.

Operations

Add operations sparingly. First, make sure that an fitting operation does not already exist. If a new operation is truly
necessary, make sure to document in a comment exactly what the operation allows. This prevents us from managing an
explosion of overlapping or contradictory permissions.

Permissions

A permission is a combination of a role, resource, and operation.

        rbac.RolePermissions{
		      SiteAdmin: {
			      Workspaces: rbac.Operations{Create}
          },
        }

This permission gives the SiteAdmin role permission to create Workspaces.

Enforcers

Enforcers are lookup tables for permissions. Different enforcers can be used to scope permissions. For example, you can
use a SiteEnforcer to enforce permissions on the site level while an OrganizationEnforcer enforces them on the
organization level.

Inheritances

Roles can inherit from other roles. For now, we do not chain inheritances. So, you will have to explicitly add links.

        Inheritances: rbac.Inheritances{
		      Admin: rbac.Roles{Member},
          SuperAdmin: rbac.Roles{Admin, Member},
	      }

SuperAdmin inherits from Admin, but must also list out its relation to Member. This may change in the future.

Note

Roles, resources, operations and enforcers should live in the same file.
*/
package rbac
