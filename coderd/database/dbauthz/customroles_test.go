package dbauthz_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestUpsertCustomRoles verifies creating custom roles cannot escalate permissions.
func TestUpsertCustomRoles(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	subjectFromRoles := func(roles rbac.ExpandableRoles) rbac.Subject {
		return rbac.Subject{
			FriendlyName: "Test user",
			ID:           userID.String(),
			Roles:        roles,
			Groups:       nil,
			Scope:        rbac.ScopeAll,
		}
	}

	canAssignRole := rbac.Role{
		Name:        "can-assign",
		DisplayName: "",
		Site: rbac.Permissions(map[string][]policy.Action{
			rbac.ResourceAssignRole.Type: {policy.ActionRead, policy.ActionCreate},
		}),
	}

	merge := func(u ...interface{}) rbac.Roles {
		all := make([]rbac.Role, 0)
		for _, v := range u {
			v := v
			switch t := v.(type) {
			case rbac.Role:
				all = append(all, t)
			case rbac.ExpandableRoles:
				all = append(all, must(t.Expand())...)
			case string:
				all = append(all, must(rbac.RoleByName(t)))
			default:
				panic("unknown type")
			}
		}

		return all
	}

	orgID := uuid.NullUUID{
		UUID:  uuid.New(),
		Valid: true,
	}
	testCases := []struct {
		name string

		subject rbac.ExpandableRoles

		// Perms to create on new custom role
		organizationID uuid.NullUUID
		site           []codersdk.Permission
		org            []codersdk.Permission
		user           []codersdk.Permission
		errorContains  string
	}{
		{
			// No roles, so no assign role
			name:          "no-roles",
			subject:       rbac.RoleNames([]string{}),
			errorContains: "forbidden",
		},
		{
			// This works because the new role has 0 perms
			name:    "empty",
			subject: merge(canAssignRole),
		},
		{
			name:           "mixed-scopes",
			subject:        merge(canAssignRole, rbac.RoleOwner()),
			organizationID: orgID,
			site: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			org: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			errorContains: "cannot assign both org and site permissions",
		},
		{
			name:    "invalid-action",
			subject: merge(canAssignRole, rbac.RoleOwner()),
			site: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				// Action does not go with resource
				codersdk.ResourceWorkspace: {codersdk.ActionViewInsights},
			}),
			errorContains: "invalid action",
		},
		{
			name:    "invalid-resource",
			subject: merge(canAssignRole, rbac.RoleOwner()),
			site: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				"foobar": {codersdk.ActionViewInsights},
			}),
			errorContains: "invalid resource",
		},
		{
			// Not allowing these at this time.
			name:    "negative-permission",
			subject: merge(canAssignRole, rbac.RoleOwner()),
			site: []codersdk.Permission{
				{
					Negate:       true,
					ResourceType: codersdk.ResourceWorkspace,
					Action:       codersdk.ActionRead,
				},
			},
			errorContains: "no negative permissions",
		},
		{
			name:    "wildcard", // not allowed
			subject: merge(canAssignRole, rbac.RoleOwner()),
			site: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {"*"},
			}),
			errorContains: "no wildcard symbols",
		},
		// escalation checks
		{
			name:    "read-workspace-escalation",
			subject: merge(canAssignRole),
			site: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			errorContains: "not allowed to grant this permission",
		},
		{
			name: "read-workspace-outside-org",
			organizationID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
			subject: merge(canAssignRole, rbac.ScopedRoleOrgAdmin(orgID.UUID)),
			org: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			errorContains: "not allowed to grant this permission",
		},
		{
			name: "user-escalation",
			// These roles do not grant user perms
			subject: merge(canAssignRole, rbac.ScopedRoleOrgAdmin(orgID.UUID)),
			user: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			errorContains: "not allowed to grant this permission",
		},
		{
			name:    "template-admin-escalation",
			subject: merge(canAssignRole, rbac.RoleTemplateAdmin()),
			site: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace:        {codersdk.ActionRead},   // ok!
				codersdk.ResourceDeploymentConfig: {codersdk.ActionUpdate}, // not ok!
			}),
			user: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead}, // ok!
			}),
			errorContains: "deployment_config",
		},
		// ok!
		{
			name:    "read-workspace-template-admin",
			subject: merge(canAssignRole, rbac.RoleTemplateAdmin()),
			site: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
		},
		{
			name:           "read-workspace-in-org",
			subject:        merge(canAssignRole, rbac.ScopedRoleOrgAdmin(orgID.UUID)),
			organizationID: orgID,
			org: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
		},
		{
			name: "user-perms",
			// This is weird, but is ok
			subject: merge(canAssignRole, rbac.RoleMember()),
			user: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
		},
		{
			name:    "site+user-perms",
			subject: merge(canAssignRole, rbac.RoleMember(), rbac.RoleTemplateAdmin()),
			site: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			user: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := dbmem.New()
			rec := &coderdtest.RecordingAuthorizer{
				Wrapped: rbac.NewAuthorizer(prometheus.NewRegistry()),
			}
			az := dbauthz.New(db, rec, slog.Make(), coderdtest.AccessControlStorePointer())

			subject := subjectFromRoles(tc.subject)
			ctx := testutil.Context(t, testutil.WaitMedium)
			ctx = dbauthz.As(ctx, subject)

			_, err := az.UpsertCustomRole(ctx, database.UpsertCustomRoleParams{
				Name:            "test-role",
				DisplayName:     "",
				OrganizationID:  tc.organizationID,
				SitePermissions: db2sdk.List(tc.site, convertSDKPerm),
				OrgPermissions:  db2sdk.List(tc.org, convertSDKPerm),
				UserPermissions: db2sdk.List(tc.user, convertSDKPerm),
			})
			if tc.errorContains != "" {
				require.ErrorContains(t, err, tc.errorContains)
			} else {
				require.NoError(t, err)

				// Verify we can fetch the role
				roles, err := az.CustomRoles(ctx, database.CustomRolesParams{
					LookupRoles: []database.NameOrganizationPair{
						{
							Name:           "test-role",
							OrganizationID: tc.organizationID.UUID,
						},
					},
					ExcludeOrgRoles: false,
					OrganizationID:  uuid.UUID{},
				})
				require.NoError(t, err)
				require.Len(t, roles, 1)
			}
		})
	}
}

func convertSDKPerm(perm codersdk.Permission) database.CustomRolePermission {
	return database.CustomRolePermission{
		Negate:       perm.Negate,
		ResourceType: string(perm.ResourceType),
		Action:       policy.Action(perm.Action),
	}
}
