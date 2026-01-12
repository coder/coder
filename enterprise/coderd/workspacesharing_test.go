package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac"
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
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

		client, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		memberClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		settings, err := memberClient.WorkspaceSharingSettings(ctx, first.OrganizationID.String())
		require.NoError(t, err)
		require.False(t, settings.SharingDisabled)
	})

	t.Run("DisabledTogglePersists", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

		client, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID))
		settings, err := orgAdminClient.PatchWorkspaceSharingSettings(ctx, first.OrganizationID.String(), codersdk.WorkspaceSharingSettings{
			SharingDisabled: true,
		})
		require.NoError(t, err)
		require.True(t, settings.SharingDisabled)

		settings, err = orgAdminClient.WorkspaceSharingSettings(ctx, first.OrganizationID.String())
		require.NoError(t, err)
		require.True(t, settings.SharingDisabled)

		settings, err = orgAdminClient.PatchWorkspaceSharingSettings(ctx, first.OrganizationID.String(), codersdk.WorkspaceSharingSettings{
			SharingDisabled: false,
		})
		require.NoError(t, err)
		require.False(t, settings.SharingDisabled)
	})

	t.Run("UpdateAuthz", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

		client, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		memberClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		_, err := memberClient.PatchWorkspaceSharingSettings(ctx, first.OrganizationID.String(), codersdk.WorkspaceSharingSettings{
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
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

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
		_, err := orgAdminClient.PatchWorkspaceSharingSettings(ctx, first.OrganizationID.String(), codersdk.WorkspaceSharingSettings{
			SharingDisabled: true,
		})
		require.NoError(t, err)

		require.Len(t, auditor.AuditLogs(), 1)
		alog := auditor.AuditLogs()[0]
		require.Equal(t, database.AuditActionWrite, alog.Action)
		require.Equal(t, database.ResourceTypeOrganization, alog.ResourceType)
		require.Equal(t, first.OrganizationID, alog.ResourceID)
	})

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()

		// Note: NOT setting the experiment flag.
		client, first := coderdenttest.New(t, &coderdenttest.Options{})

		ctx := testutil.Context(t, testutil.WaitMedium)

		memberClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		_, err := memberClient.WorkspaceSharingSettings(ctx, first.OrganizationID.String())
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "requires enabling")
		require.Contains(t, apiErr.Message, "workspace-sharing")
	})
}

func TestWorkspaceSharingDisabled(t *testing.T) {
	t.Parallel()

	t.Run("ACLEndpointsForbidden", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

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
		_, err := orgAdminClient.PatchWorkspaceSharingSettings(ctx, owner.OrganizationID.String(), codersdk.WorkspaceSharingSettings{
			SharingDisabled: true,
		})
		require.NoError(t, err)

		// We don't disallow reading the ACL, but the response should still be
		// an authz error due to the lack of workspace:share permission.
		_, err = workspaceOwnerClient.WorkspaceACL(ctx, ws.ID)
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode())
		require.Contains(t, apiErr.Detail, "unauthorized")

		// We don't allow mutating the ACL.
		assertSharingDisabled := func(t *testing.T, err error) {
			t.Helper()

			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
			require.Equal(t, "Workspace sharing is disabled for this organization.", apiErr.Message)
		}

		err = workspaceOwnerClient.UpdateWorkspaceACL(ctx, ws.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				uuid.NewString(): codersdk.WorkspaceRoleUse,
			},
		})
		assertSharingDisabled(t, err)

		err = workspaceOwnerClient.DeleteWorkspaceACL(ctx, ws.ID)
		assertSharingDisabled(t, err)
	})

	t.Run("ACLsPurged", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

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
		_, err = orgAdminClient.PatchWorkspaceSharingSettings(ctx, owner.OrganizationID.String(), codersdk.WorkspaceSharingSettings{
			SharingDisabled: true,
		})
		require.NoError(t, err)

		_, err = orgAdminClient.PatchWorkspaceSharingSettings(ctx, owner.OrganizationID.String(), codersdk.WorkspaceSharingSettings{
			SharingDisabled: false,
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
}
