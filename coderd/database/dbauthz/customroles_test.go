package dbauthz_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
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
			rbac.ResourceAssignRole.Type: {policy.ActionCreate},
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

	orgID := uuid.New()
	testCases := []struct {
		name string

		subject rbac.ExpandableRoles

		// Perms to create on new custom role
		site          []rbac.Permission
		org           map[string][]rbac.Permission
		user          []rbac.Permission
		errorContains string
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
			name:    "mixed-scopes",
			subject: merge(canAssignRole, rbac.RoleOwner()),
			site: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceWorkspace.Type: {policy.ActionRead},
			}),
			org: map[string][]rbac.Permission{
				uuid.New().String(): rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceWorkspace.Type: {policy.ActionRead},
				}),
			},
			errorContains: "cannot assign both org and site permissions",
		},
		{
			name:    "multiple-org",
			subject: merge(canAssignRole, rbac.RoleOwner()),
			org: map[string][]rbac.Permission{
				uuid.New().String(): rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceWorkspace.Type: {policy.ActionRead},
				}),
				uuid.New().String(): rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceWorkspace.Type: {policy.ActionRead},
				}),
			},
			errorContains: "cannot assign permissions to more than 1",
		},
		{
			name:    "invalid-action",
			subject: merge(canAssignRole, rbac.RoleOwner()),
			site: rbac.Permissions(map[string][]policy.Action{
				// Action does not go with resource
				rbac.ResourceWorkspace.Type: {policy.ActionViewInsights},
			}),
			errorContains: "invalid action",
		},
		{
			name:    "invalid-resource",
			subject: merge(canAssignRole, rbac.RoleOwner()),
			site: rbac.Permissions(map[string][]policy.Action{
				"foobar": {policy.ActionViewInsights},
			}),
			errorContains: "invalid resource",
		},
		{
			// Not allowing these at this time.
			name:    "negative-permission",
			subject: merge(canAssignRole, rbac.RoleOwner()),
			site: []rbac.Permission{
				{
					Negate:       true,
					ResourceType: rbac.ResourceWorkspace.Type,
					Action:       policy.ActionRead,
				},
			},
			errorContains: "no negative permissions",
		},
		{
			name:    "wildcard", // not allowed
			subject: merge(canAssignRole, rbac.RoleOwner()),
			site: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceWorkspace.Type: {policy.WildcardSymbol},
			}),
			errorContains: "no wildcard symbols",
		},
		// escalation checks
		{
			name:    "read-workspace-escalation",
			subject: merge(canAssignRole),
			site: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceWorkspace.Type: {policy.ActionRead},
			}),
			errorContains: "not allowed to grant this permission",
		},
		{
			name:    "read-workspace-outside-org",
			subject: merge(canAssignRole, rbac.RoleOrgAdmin(orgID)),
			org: map[string][]rbac.Permission{
				// The org admin is for a different org
				uuid.NewString(): rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceWorkspace.Type: {policy.ActionRead},
				}),
			},
			errorContains: "not allowed to grant this permission",
		},
		{
			name: "user-escalation",
			// These roles do not grant user perms
			subject: merge(canAssignRole, rbac.RoleOrgAdmin(orgID)),
			user: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceWorkspace.Type: {policy.ActionRead},
			}),
			errorContains: "not allowed to grant this permission",
		},
		{
			name:    "template-admin-escalation",
			subject: merge(canAssignRole, rbac.RoleTemplateAdmin()),
			site: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceWorkspace.Type:        {policy.ActionRead},   // ok!
				rbac.ResourceDeploymentConfig.Type: {policy.ActionUpdate}, // not ok!
			}),
			user: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceWorkspace.Type: {policy.ActionRead}, // ok!
			}),
			errorContains: "deployment_config",
		},
		// ok!
		{
			name:    "read-workspace-template-admin",
			subject: merge(canAssignRole, rbac.RoleTemplateAdmin()),
			site: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceWorkspace.Type: {policy.ActionRead},
			}),
		},
		{
			name:    "read-workspace-in-org",
			subject: merge(canAssignRole, rbac.RoleOrgAdmin(orgID)),
			org: map[string][]rbac.Permission{
				// Org admin of this org, this is ok!
				orgID.String(): rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceWorkspace.Type: {policy.ActionRead},
				}),
			},
		},
		{
			name: "user-perms",
			// This is weird, but is ok
			subject: merge(canAssignRole, rbac.RoleMember()),
			user: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceWorkspace.Type: {policy.ActionRead},
			}),
		},
		{
			name:    "site+user-perms",
			subject: merge(canAssignRole, rbac.RoleMember(), rbac.RoleTemplateAdmin()),
			site: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceWorkspace.Type: {policy.ActionRead},
			}),
			user: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceWorkspace.Type: {policy.ActionRead},
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
				SitePermissions: must(json.Marshal(tc.site)),
				OrgPermissions:  must(json.Marshal(tc.org)),
				UserPermissions: must(json.Marshal(tc.user)),
			})
			if tc.errorContains != "" {
				require.ErrorContains(t, err, tc.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
