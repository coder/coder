package coderd_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceSharingSettings(t *testing.T) {
	t.Parallel()

	t.Run("DisabledDefaultsFalse", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)

		client, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		// Use a regular user to make sure the setting is exposed to them.
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		settings, err := memberClient.WorkspaceSharingSettings(ctx, first.OrganizationID.String())
		require.NoError(t, err)
		// Check the deprecated boolean field.
		require.False(t, settings.SharingDisabled)
		require.Equal(t, codersdk.ShareableWorkspaceOwnersEveryone, settings.ShareableWorkspaceOwners)
	})

	t.Run("DisabledTogglePersists", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)

		client, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID))

		// Disable sharing via the deprecated boolean field.
		settings, err := orgAdminClient.PatchWorkspaceSharingSettings(ctx, first.OrganizationID.String(), codersdk.UpdateWorkspaceSharingSettingsRequest{
			SharingDisabled: true,
		})
		require.NoError(t, err)
		require.True(t, settings.SharingDisabled)
		require.Equal(t, codersdk.ShareableWorkspaceOwnersNone, settings.ShareableWorkspaceOwners)

		settings, err = orgAdminClient.WorkspaceSharingSettings(ctx, first.OrganizationID.String())
		require.NoError(t, err)
		require.True(t, settings.SharingDisabled)
		require.Equal(t, codersdk.ShareableWorkspaceOwnersNone, settings.ShareableWorkspaceOwners)

		// Switch to service_accounts mode via the new field.
		settings, err = orgAdminClient.PatchWorkspaceSharingSettings(ctx, first.OrganizationID.String(), codersdk.UpdateWorkspaceSharingSettingsRequest{
			ShareableWorkspaceOwners: codersdk.ShareableWorkspaceOwnersServiceAccounts,
		})
		require.NoError(t, err)
		require.False(t, settings.SharingDisabled)
		require.Equal(t, codersdk.ShareableWorkspaceOwnersServiceAccounts, settings.ShareableWorkspaceOwners)

		settings, err = orgAdminClient.WorkspaceSharingSettings(ctx, first.OrganizationID.String())
		require.NoError(t, err)
		require.Equal(t, codersdk.ShareableWorkspaceOwnersServiceAccounts, settings.ShareableWorkspaceOwners)

		// Re-enable full sharing.
		settings, err = orgAdminClient.PatchWorkspaceSharingSettings(ctx, first.OrganizationID.String(), codersdk.UpdateWorkspaceSharingSettingsRequest{
			ShareableWorkspaceOwners: codersdk.ShareableWorkspaceOwnersEveryone,
		})
		require.NoError(t, err)
		require.False(t, settings.SharingDisabled)
		require.Equal(t, codersdk.ShareableWorkspaceOwnersEveryone, settings.ShareableWorkspaceOwners)

		settings, err = orgAdminClient.WorkspaceSharingSettings(ctx, first.OrganizationID.String())
		require.NoError(t, err)
		require.Equal(t, codersdk.ShareableWorkspaceOwnersEveryone, settings.ShareableWorkspaceOwners)
	})

	t.Run("InvalidValueRejected", func(t *testing.T) {
		t.Parallel()

		client, first := coderdenttest.New(t, nil)

		ctx := testutil.Context(t, testutil.WaitMedium)

		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID))
		_, err := orgAdminClient.PatchWorkspaceSharingSettings(ctx, first.OrganizationID.String(), codersdk.UpdateWorkspaceSharingSettingsRequest{
			ShareableWorkspaceOwners: "invalid",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("UpdateAuthz", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)

		client, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		memberClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		_, err := memberClient.PatchWorkspaceSharingSettings(ctx, first.OrganizationID.String(), codersdk.UpdateWorkspaceSharingSettingsRequest{
			SharingDisabled: true,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
	})

	t.Run("AuditLog", func(t *testing.T) {
		t.Parallel()

		auditor := audit.NewMock()
		dv := coderdtest.DeploymentValues(t)

		client, first := coderdenttest.New(t, &coderdenttest.Options{
			AuditLogging: true,
			Options: &coderdtest.Options{
				DeploymentValues: dv,
				Auditor:          auditor,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAuditLog: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID))
		auditor.ResetLogs()
		_, err := orgAdminClient.PatchWorkspaceSharingSettings(ctx, first.OrganizationID.String(), codersdk.UpdateWorkspaceSharingSettingsRequest{
			SharingDisabled: true,
		})
		require.NoError(t, err)

		require.Len(t, auditor.AuditLogs(), 1)
		alog := auditor.AuditLogs()[0]
		require.Equal(t, database.AuditActionWrite, alog.Action)
		require.Equal(t, database.ResourceTypeOrganization, alog.ResourceType)
		require.Equal(t, first.OrganizationID, alog.ResourceID)
	})
}

func TestWorkspaceSharingDisabled(t *testing.T) {
	t.Parallel()

	t.Run("ACLEndpointsForbidden", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)

		client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
		})

		workspaceOwnerClient, workspaceOwner := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ws := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        workspaceOwner.ID,
			OrganizationID: owner.OrganizationID,
		}).Do().Workspace

		ctx := testutil.Context(t, testutil.WaitMedium)

		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
		_, err := orgAdminClient.PatchWorkspaceSharingSettings(ctx, owner.OrganizationID.String(), codersdk.UpdateWorkspaceSharingSettingsRequest{
			ShareableWorkspaceOwners: codersdk.ShareableWorkspaceOwnersNone,
		})
		require.NoError(t, err)

		// Reading the ACL list remains allowed even when workspace sharing is
		// disabled, but mutating it is forbidden.
		_, err = workspaceOwnerClient.WorkspaceACL(ctx, ws.ID)
		require.NoError(t, err)

		// We don't allow mutating the ACL.
		assertSharingDisabled := func(t *testing.T, err error) {
			t.Helper()

			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
			require.Equal(t, "Workspace sharing is disabled for this organization.", apiErr.Message)
		}

		// Despite the site-wide workspace.share permission for the owner,
		// the endpoint should return an authz error.
		err = client.UpdateWorkspaceACL(ctx, ws.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				uuid.NewString(): codersdk.WorkspaceRoleUse,
			},
		})
		assertSharingDisabled(t, err)

		err = workspaceOwnerClient.DeleteWorkspaceACL(ctx, ws.ID)
		assertSharingDisabled(t, err)
	})

	t.Run("ACLEndpointsForbiddenServiceAccountsMode", func(t *testing.T) {
		t.Parallel()

		client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureServiceAccounts: 1,
				},
			},
		})

		regularClient, regularUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		regularWS := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        regularUser.ID,
			OrganizationID: owner.OrganizationID,
		}).Do().Workspace

		// Create an SA with a workspace.
		saClient, saUser := coderdtest.CreateAnotherUserMutators(t, client, owner.OrganizationID, nil, func(r *codersdk.CreateUserRequestWithOrgs) {
			r.ServiceAccount = true
		})
		saWS := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        saUser.ID,
			OrganizationID: owner.OrganizationID,
		}).Do().Workspace

		ctx := testutil.Context(t, testutil.WaitMedium)

		orgAdminClient, orgAdmin := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
		_, err := orgAdminClient.PatchWorkspaceSharingSettings(ctx, owner.OrganizationID.String(), codersdk.UpdateWorkspaceSharingSettingsRequest{
			ShareableWorkspaceOwners: codersdk.ShareableWorkspaceOwnersServiceAccounts,
		})
		require.NoError(t, err)

		// Regular member cannot share their own workspace.
		err = regularClient.UpdateWorkspaceACL(ctx, regularWS.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				orgAdmin.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())

		// SA can share their own workspace.
		err = saClient.UpdateWorkspaceACL(ctx, saWS.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				regularUser.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})
		require.NoError(t, err)
	})

	// Future-proofing: if custom roles with member-scoped
	// workspace:share are ever allowed, the member-level negation
	// from the organization-member system role must block sharing in
	// service_accounts mode even with such custom role.
	t.Run("MemberCannotBypassWithCustomRole", func(t *testing.T) {
		t.Parallel()

		rawDB, pubsub, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		client, _, _, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: rawDB,
				Pubsub:   pubsub,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles:  1,
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		// Create an empty custom role via the API, then add
		// member-scoped workspace:share via raw SQL (the API and
		// dbauthz both reject member permissions on custom roles).
		//nolint:gocritic // owner context required for role creation
		customRole, err := client.CreateOrganizationRole(ctx, codersdk.Role{
			Name:           "workspace-share-granter",
			OrganizationID: owner.OrganizationID.String(),
		})
		require.NoError(t, err)

		_, err = sqlDB.ExecContext(ctx,
			`UPDATE custom_roles SET member_permissions = $1 WHERE name = $2 AND organization_id = $3`,
			database.CustomRolePermissions{{
				ResourceType: rbac.ResourceWorkspace.Type,
				Action:       policy.ActionShare,
			}},
			customRole.Name,
			owner.OrganizationID,
		)
		require.NoError(t, err)

		// Create a member and assign the custom role.
		memberClient, memberUser := coderdtest.CreateAnotherUserMutators(
			t, client, owner.OrganizationID,
			[]rbac.RoleIdentifier{{
				Name:           customRole.Name,
				OrganizationID: owner.OrganizationID,
			}},
		)
		memberWS := dbfake.WorkspaceBuild(t, rawDB, database.WorkspaceTable{
			OwnerID:        memberUser.ID,
			OrganizationID: owner.OrganizationID,
		}).Do().Workspace

		_, sharedUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Switch to service_accounts mode.
		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
		_, err = orgAdminClient.PatchWorkspaceSharingSettings(ctx, owner.OrganizationID.String(), codersdk.UpdateWorkspaceSharingSettingsRequest{
			ShareableWorkspaceOwners: codersdk.ShareableWorkspaceOwnersServiceAccounts,
		})
		require.NoError(t, err)

		// Despite the custom role granting workspace:share at the
		// member level, the negation from organization-member should
		// block it.
		err = memberClient.UpdateWorkspaceACL(ctx, memberWS.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				sharedUser.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
	})

	t.Run("ACLsPurged", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)

		client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})

		workspaceOwnerClient, workspaceOwner := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		_, sharedUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Create a group to test group ACL purging.
		group := coderdtest.CreateGroup(t, client, owner.OrganizationID, "test-group")

		ws := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        workspaceOwner.ID,
			OrganizationID: owner.OrganizationID,
		}).Do().Workspace

		ctx := testutil.Context(t, testutil.WaitMedium)

		// Set both user and group ACLs.
		err := workspaceOwnerClient.UpdateWorkspaceACL(ctx, ws.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				sharedUser.ID.String(): codersdk.WorkspaceRoleUse,
			},
			GroupRoles: map[string]codersdk.WorkspaceRole{
				group.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})
		require.NoError(t, err)

		acl, err := workspaceOwnerClient.WorkspaceACL(ctx, ws.ID)
		require.NoError(t, err)
		require.Len(t, acl.Users, 1)
		require.Equal(t, sharedUser.ID, acl.Users[0].ID)
		require.Equal(t, codersdk.WorkspaceRoleUse, acl.Users[0].Role)
		require.Len(t, acl.Groups, 1)
		require.Equal(t, group.ID, acl.Groups[0].ID)
		require.Equal(t, codersdk.WorkspaceRoleUse, acl.Groups[0].Role)

		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
		_, err = orgAdminClient.PatchWorkspaceSharingSettings(ctx, owner.OrganizationID.String(), codersdk.UpdateWorkspaceSharingSettingsRequest{
			ShareableWorkspaceOwners: codersdk.ShareableWorkspaceOwnersNone,
		})
		require.NoError(t, err)

		_, err = orgAdminClient.PatchWorkspaceSharingSettings(ctx, owner.OrganizationID.String(), codersdk.UpdateWorkspaceSharingSettingsRequest{
			ShareableWorkspaceOwners: codersdk.ShareableWorkspaceOwnersEveryone,
		})
		require.NoError(t, err)

		// Verify both user and group ACLs are purged.
		acl, err = workspaceOwnerClient.WorkspaceACL(ctx, ws.ID)
		require.NoError(t, err)
		require.Empty(t, acl.Users)
		require.Empty(t, acl.Groups)

		// Verify ACLs can be set again after re-enabling sharing.
		err = workspaceOwnerClient.UpdateWorkspaceACL(ctx, ws.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				sharedUser.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})
		require.NoError(t, err)
		acl, err = workspaceOwnerClient.WorkspaceACL(ctx, ws.ID)
		require.NoError(t, err)
		require.Len(t, acl.Users, 1)
		require.Equal(t, sharedUser.ID, acl.Users[0].ID)
	})

	t.Run("ACLsPurgedExceptServiceAccounts", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)

		client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC:    1,
					codersdk.FeatureServiceAccounts: 1,
				},
			},
		})

		// Regular user with a workspace.
		workspaceOwnerClient, workspaceOwner := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		_, sharedUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		regularWS := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        workspaceOwner.ID,
			OrganizationID: owner.OrganizationID,
		}).Do().Workspace

		// Service account with a workspace.
		_, saUser := coderdtest.CreateAnotherUserMutators(t, client, owner.OrganizationID, nil, func(r *codersdk.CreateUserRequestWithOrgs) {
			r.ServiceAccount = true
		})
		saWS := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        saUser.ID,
			OrganizationID: owner.OrganizationID,
		}).Do().Workspace

		ctx := testutil.Context(t, testutil.WaitMedium)

		// Share regular user's workspace with sharedUser.
		err := workspaceOwnerClient.UpdateWorkspaceACL(ctx, regularWS.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				sharedUser.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})
		require.NoError(t, err)

		// Use the owner client (site admin) to share the SA workspace,
		// since the SA can't authenticate via the API.
		err = client.UpdateWorkspaceACL(ctx, saWS.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				sharedUser.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})
		require.NoError(t, err)

		// Switch to service_accounts mode.
		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
		_, err = orgAdminClient.PatchWorkspaceSharingSettings(ctx, owner.OrganizationID.String(), codersdk.UpdateWorkspaceSharingSettingsRequest{
			ShareableWorkspaceOwners: codersdk.ShareableWorkspaceOwnersServiceAccounts,
		})
		require.NoError(t, err)

		// Regular user workspace ACLs should be purged.
		acl, err := workspaceOwnerClient.WorkspaceACL(ctx, regularWS.ID)
		require.NoError(t, err)
		require.Empty(t, acl.Users)

		// Service account workspace ACLs should be preserved.
		acl, err = client.WorkspaceACL(ctx, saWS.ID)
		require.NoError(t, err)
		require.Len(t, acl.Users, 1)
		require.Equal(t, sharedUser.ID, acl.Users[0].ID)
	})
}

// TestWorkspaceSharingDormancySurvivesACL asserts the DESIRED behavior
// for shared workspaces that go dormant: ACL recipients keep access to
// the dormant workspace (bounded by the dormant action set), an
// admin-role recipient can wake it, and the list and direct-access
// views agree.
//
// The test is skipped because DormantRBAC()
// (coderd/database/modelmethods.go) currently attaches no ACLs: ACL
// recipients get 404 on direct access and on wake attempts while the
// dormant workspace still appears in their list, which is filtered
// against the non-dormant workspace type and the row's intact ACL
// columns. Remove the skip when DormantRBAC() carries the workspace
// ACLs.
func TestWorkspaceSharingDormancySurvivesACL(t *testing.T) {
	t.Parallel()
	t.Skip("known bug: DormantRBAC() attaches no ACLs, so shared access does not survive dormancy")

	client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	ctx := testutil.Context(t, testutil.WaitMedium)

	wsOwnerClient, wsOwner := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	ws := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        wsOwner.ID,
		OrganizationID: owner.OrganizationID,
	}).Do().Workspace

	recipientClient, recipient := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// Share with the admin role, the strongest ACL grant.
	err := wsOwnerClient.UpdateWorkspaceACL(ctx, ws.ID, codersdk.UpdateWorkspaceACL{
		UserRoles: map[string]codersdk.WorkspaceRole{
			recipient.ID.String(): codersdk.WorkspaceRoleAdmin,
		},
	})
	require.NoError(t, err)

	// The recipient can access the active workspace.
	_, err = recipientClient.Workspace(ctx, ws.ID)
	require.NoError(t, err)

	// The owner marks the workspace dormant.
	err = wsOwnerClient.UpdateWorkspaceDormancy(ctx, ws.ID, codersdk.UpdateWorkspaceDormancy{Dormant: true})
	require.NoError(t, err)

	// The recipient keeps read access to the dormant workspace, matching
	// what the list already shows them.
	_, err = recipientClient.Workspace(ctx, ws.ID)
	require.NoError(t, err)
	listed, err := recipientClient.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.True(t, slices.ContainsFunc(listed.Workspaces, func(w codersdk.Workspace) bool {
		return w.ID == ws.ID
	}), "dormant shared workspace should appear in the recipient's list")

	// An admin-role recipient holds update, so they can wake the
	// workspace before the dormancy policy deletes it.
	err = recipientClient.UpdateWorkspaceDormancy(ctx, ws.ID, codersdk.UpdateWorkspaceDormancy{Dormant: false})
	require.NoError(t, err)

	_, err = recipientClient.Workspace(ctx, ws.ID)
	require.NoError(t, err)
}

// TestWorkspaceSharingPartialCapability verifies that a share to a
// recipient whose roles carry only some of the granted actions is
// accepted, and that the entry then has per-action effect: actions the
// recipient's member-level permissions carry work, the rest are denied
// at access time.
//
// The partial recipient is a regular org member with a custom role that
// negates workspace update at the member level. Negation is used instead
// of the MinimumImplicitMember experiment because the experiment's rbac
// global is reset by concurrently constructed coderd test servers.
func TestWorkspaceSharingPartialCapability(t *testing.T) {
	t.Parallel()

	client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	ctx := testutil.Context(t, testutil.WaitMedium)

	wsOwnerClient, wsOwner := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	ws := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        wsOwner.ID,
		OrganizationID: owner.OrganizationID,
	}).Do().Workspace

	recipientClient, recipient := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// A member-level negation vetoes the org for that action in the ACL
	// precondition, so the recipient's capability set is every workspace
	// action except update. Member-key permissions are only storable on
	// system roles (dbauthz customRoleCheck), so the ban is inserted as
	// one, mirroring how workspace capability roles are provisioned.
	//nolint:gocritic // Inserting the custom role directly requires system access.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	updateBan, err := db.InsertCustomRole(sysCtx, database.InsertCustomRoleParams{
		Name:           "workspace-update-ban",
		DisplayName:    "Workspace Update Ban",
		OrganizationID: uuid.NullUUID{UUID: owner.OrganizationID, Valid: true},
		IsSystem:       true,
		MemberPermissions: []database.CustomRolePermission{{
			Negate:       true,
			ResourceType: rbac.ResourceWorkspace.Type,
			Action:       policy.ActionUpdate,
		}},
	})
	require.NoError(t, err)
	_, err = db.UpdateMemberRoles(sysCtx, database.UpdateMemberRolesParams{
		GrantedRoles: []string{updateBan.Name},
		UserID:       recipient.ID,
		OrgID:        owner.OrganizationID,
	})
	require.NoError(t, err)

	// The admin role's action set includes update, which the recipient
	// cannot receive. The share is still accepted because the remaining
	// actions are effective.
	err = wsOwnerClient.UpdateWorkspaceACL(ctx, ws.ID, codersdk.UpdateWorkspaceACL{
		UserRoles: map[string]codersdk.WorkspaceRole{
			recipient.ID.String(): codersdk.WorkspaceRoleAdmin,
		},
	})
	require.NoError(t, err)

	// Granted actions the recipient's roles carry are effective.
	_, err = recipientClient.Workspace(ctx, ws.ID)
	require.NoError(t, err)
	listed, err := recipientClient.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.True(t, slices.ContainsFunc(listed.Workspaces, func(w codersdk.Workspace) bool {
		return w.ID == ws.ID
	}), "shared workspace should appear in the recipient's list")

	// The banned action is denied at access time even though the ACL
	// entry grants it. Autostart updates are gated on workspace update
	// alone.
	err = recipientClient.UpdateWorkspaceAutostart(ctx, ws.ID, codersdk.UpdateWorkspaceAutostartRequest{
		Schedule: ptr.Ref("CRON_TZ=UTC 30 9 * * 1-5"),
	})
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusNotFound, apiErr.StatusCode())

	// The workspace owner's own update capability is unaffected; the
	// denial above is the recipient's negation, not an endpoint problem.
	err = wsOwnerClient.UpdateWorkspaceAutostart(ctx, ws.ID, codersdk.UpdateWorkspaceAutostartRequest{
		Schedule: ptr.Ref("CRON_TZ=UTC 30 9 * * 1-5"),
	})
	require.NoError(t, err)
}

// TestWorkspaceSharingCapabilityPrecondition verifies that workspace ACL
// grants require the recipient's roles to carry the granted actions at
// the member level in the workspace's organization: recipients with no
// matching actions are rejected at share time, and losing the actions
// later revokes the shared access even though the ACL entry remains.
//
// The recipient without capability is a member of a different org: a
// same-org member without workspace actions requires the
// MinimumImplicitMember experiment, whose rbac global is reset by any
// concurrently constructed coderd test server, so that scenario is
// covered by the rbac package (TestAuthorizeACLCapabilityPrecondition)
// instead.
func TestWorkspaceSharingCapabilityPrecondition(t *testing.T) {
	t.Parallel()

	client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	ctx := testutil.Context(t, testutil.WaitMedium)

	secondOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

	wsOwnerClient, wsOwner := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	ws := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        wsOwner.ID,
		OrganizationID: owner.OrganizationID,
	}).Do().Workspace

	// A member of another org holds no member-level workspace actions in
	// the workspace's org.
	_, otherOrgUser := coderdtest.CreateAnotherUser(t, client, secondOrg.ID)
	fullClient, fullUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// Sharing with a recipient holding no workspace actions in this org
	// is rejected at share time.
	err := wsOwnerClient.UpdateWorkspaceACL(ctx, ws.ID, codersdk.UpdateWorkspaceACL{
		UserRoles: map[string]codersdk.WorkspaceRole{
			otherOrgUser.ID.String(): codersdk.WorkspaceRoleUse,
		},
	})
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	require.Contains(t, apiErr.Message, "Invalid request to update workspace ACL")

	// Sharing with a workspace-capable member succeeds and grants access.
	err = wsOwnerClient.UpdateWorkspaceACL(ctx, ws.ID, codersdk.UpdateWorkspaceACL{
		UserRoles: map[string]codersdk.WorkspaceRole{
			fullUser.ID.String(): codersdk.WorkspaceRoleUse,
		},
	})
	require.NoError(t, err)
	_, err = fullClient.Workspace(ctx, ws.ID)
	require.NoError(t, err)
	// The list endpoint authorizes via the compiled SQL filter rather
	// than per-object checks; the shared workspace must appear there too.
	listed, err := fullClient.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.True(t, slices.ContainsFunc(listed.Workspaces, func(w codersdk.Workspace) bool {
		return w.ID == ws.ID
	}), "shared workspace should appear in the recipient's list")

	// Removing the recipient from the organization removes their
	// member-level actions there, revoking the shared access at
	// authorization time; the ACL entry remains but is inert.
	// Membership in the second org keeps the account otherwise valid.
	_, err = client.PostOrganizationMember(ctx, secondOrg.ID, fullUser.ID.String())
	require.NoError(t, err)
	//nolint:gocritic // Only owners can remove org members.
	err = client.DeleteOrganizationMember(ctx, owner.OrganizationID, fullUser.ID.String())
	require.NoError(t, err)

	_, err = fullClient.Workspace(ctx, ws.ID)
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	// The SQL filter must exclude it as well.
	listed, err = fullClient.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.False(t, slices.ContainsFunc(listed.Workspaces, func(w codersdk.Workspace) bool {
		return w.ID == ws.ID
	}), "workspace should not appear in the list after the capability is revoked")
}
