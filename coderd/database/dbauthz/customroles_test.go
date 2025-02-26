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

// TestInsertCustomRoles verifies creating custom roles cannot escalate permissions.
func TestInsertCustomRoles(t *testing.T) {
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

	canCreateCustomRole := rbac.Role{
		Identifier:  rbac.RoleIdentifier{Name: "can-assign"},
		DisplayName: "",
		Site: rbac.Permissions(map[string][]policy.Action{
			rbac.ResourceAssignOrgRole.Type: {policy.ActionRead, policy.ActionCreate},
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
			case rbac.RoleIdentifier:
				all = append(all, must(rbac.RoleByName(t)))
			default:
				panic("unknown type")
			}
		}

		return all
	}

	orgID := uuid.New()

	testCases := []struct {
		name string

		subject rbac.ExpandableRoles

		// Perms to create on new custom role
		organizationID uuid.UUID
		site           []codersdk.Permission
		org            []codersdk.Permission
		user           []codersdk.Permission
		errorContains  string
	}{
		{
			// No roles, so no assign role
			name:           "no-roles",
			organizationID: orgID,
			subject:        rbac.RoleIdentifiers{},
			errorContains:  "forbidden",
		},
		{
			// This works because the new role has 0 perms
			name:           "empty",
			organizationID: orgID,
			subject:        merge(canCreateCustomRole),
		},
		{
			name:           "mixed-scopes",
			organizationID: orgID,
			subject:        merge(canCreateCustomRole, rbac.RoleOwner()),
			site: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			org: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			errorContains: "organization roles specify site or user permissions",
		},
		{
			name:           "invalid-action",
			organizationID: orgID,
			subject:        merge(canCreateCustomRole, rbac.RoleOwner()),
			org: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				// Action does not go with resource
				codersdk.ResourceWorkspace: {codersdk.ActionViewInsights},
			}),
			errorContains: "invalid action",
		},
		{
			name:           "invalid-resource",
			organizationID: orgID,
			subject:        merge(canCreateCustomRole, rbac.RoleOwner()),
			org: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				"foobar": {codersdk.ActionViewInsights},
			}),
			errorContains: "invalid resource",
		},
		{
			// Not allowing these at this time.
			name:           "negative-permission",
			organizationID: orgID,
			subject:        merge(canCreateCustomRole, rbac.RoleOwner()),
			org: []codersdk.Permission{
				{
					Negate:       true,
					ResourceType: codersdk.ResourceWorkspace,
					Action:       codersdk.ActionRead,
				},
			},
			errorContains: "no negative permissions",
		},
		{
			name:           "wildcard", // not allowed
			organizationID: orgID,
			subject:        merge(canCreateCustomRole, rbac.RoleOwner()),
			org: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {"*"},
			}),
			errorContains: "no wildcard symbols",
		},
		// escalation checks
		{
			name:           "read-workspace-escalation",
			organizationID: orgID,
			subject:        merge(canCreateCustomRole),
			org: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			errorContains: "not allowed to grant this permission",
		},
		{
			name:           "read-workspace-outside-org",
			organizationID: uuid.New(),
			subject:        merge(canCreateCustomRole, rbac.ScopedRoleOrgAdmin(orgID)),
			org: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			errorContains: "not allowed to grant this permission",
		},
		{
			name: "user-escalation",
			// These roles do not grant user perms
			organizationID: orgID,
			subject:        merge(canCreateCustomRole, rbac.ScopedRoleOrgAdmin(orgID)),
			user: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			errorContains: "organization roles specify site or user permissions",
		},
		{
			name:           "site-escalation",
			organizationID: orgID,
			subject:        merge(canCreateCustomRole, rbac.RoleTemplateAdmin()),
			site: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceDeploymentConfig: {codersdk.ActionUpdate}, // not ok!
			}),
			errorContains: "organization roles specify site or user permissions",
		},
		// ok!
		{
			name:           "read-workspace-template-admin",
			organizationID: orgID,
			subject:        merge(canCreateCustomRole, rbac.RoleTemplateAdmin()),
			org: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
		},
		{
			name:           "read-workspace-in-org",
			organizationID: orgID,
			subject:        merge(canCreateCustomRole, rbac.ScopedRoleOrgAdmin(orgID)),
			org: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
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

			_, err := az.InsertCustomRole(ctx, database.InsertCustomRoleParams{
				Name:            "test-role",
				DisplayName:     "",
				OrganizationID:  uuid.NullUUID{UUID: tc.organizationID, Valid: true},
				SitePermissions: db2sdk.List(tc.site, convertSDKPerm),
				OrgPermissions:  db2sdk.List(tc.org, convertSDKPerm),
				UserPermissions: db2sdk.List(tc.user, convertSDKPerm),
			})
			if tc.errorContains != "" {
				require.ErrorContains(t, err, tc.errorContains)
			} else {
				require.NoError(t, err)

				// Verify the role is fetched with the lookup filter.
				roles, err := az.CustomRoles(ctx, database.CustomRolesParams{
					LookupRoles: []database.NameOrganizationPair{
						{
							Name:           "test-role",
							OrganizationID: tc.organizationID,
						},
					},
					ExcludeOrgRoles: false,
					OrganizationID:  uuid.Nil,
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
