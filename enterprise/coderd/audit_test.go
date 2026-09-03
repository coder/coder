package coderd_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestEnterpriseAuditLogs(t *testing.T) {
	t.Parallel()

	t.Run("IncludeOrganization", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		//nolint:gocritic // only owners can create organizations
		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "new-org",
			DisplayName: "New organization",
			Description: "A new organization to love and cherish until the test is over.",
			Icon:        "/emojis/1f48f-1f3ff.png",
		})
		require.NoError(t, err)

		err = client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			OrganizationID: o.ID,
			ResourceID:     user.UserID,
		})
		require.NoError(t, err)

		alogs, err := client.AuditLogs(ctx, codersdk.AuditLogsRequest{
			Pagination: codersdk.Pagination{
				Limit: 1,
			},
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), alogs.Count)
		require.Len(t, alogs.AuditLogs, 1)

		// Make sure the organization is fully populated.
		require.Equal(t, &codersdk.MinimalOrganization{
			ID:          o.ID,
			Name:        o.Name,
			DisplayName: o.DisplayName,
			Icon:        o.Icon,
		}, alogs.AuditLogs[0].Organization)

		// OrganizationID is deprecated, but make sure it is set.
		require.Equal(t, o.ID, alogs.AuditLogs[0].OrganizationID)

		// Delete the org and try again, should be mostly empty.
		err = client.DeleteOrganization(ctx, o.ID.String())
		require.NoError(t, err)

		alogs, err = client.AuditLogs(ctx, codersdk.AuditLogsRequest{
			Pagination: codersdk.Pagination{
				Limit: 1,
			},
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), alogs.Count)
		require.Len(t, alogs.AuditLogs, 1)

		// OrganizationID is deprecated, but make sure it is set.
		require.Equal(t, o.ID, alogs.AuditLogs[0].OrganizationID)

		// Some audit entries do not have an organization at all, in which case the
		// response omits the organization.
		err = client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			ResourceType: codersdk.ResourceTypeAPIKey,
			ResourceID:   user.UserID,
		})
		require.NoError(t, err)

		alogs, err = client.AuditLogs(ctx, codersdk.AuditLogsRequest{
			SearchQuery: "resource_type:api_key",
			Pagination: codersdk.Pagination{
				Limit: 1,
			},
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), alogs.Count)
		require.Len(t, alogs.AuditLogs, 1)

		// The other will have no organization.
		require.Equal(t, (*codersdk.MinimalOrganization)(nil), alogs.AuditLogs[0].Organization)

		// OrganizationID is deprecated, but make sure it is empty.
		require.Equal(t, uuid.Nil, alogs.AuditLogs[0].OrganizationID)
	})

	t.Run("MCPServerConfigLinkForMutationOnlyRole", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		owner, firstUser := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		config := createMCPServerConfigForOrganization(t, owner, firstUser.OrganizationID, "audit-link-mcp")
		// Remove the default everyone-in-org ACL grant so the custom
		// role's update permission is the requester's only access.
		//nolint:gocritic // Owner access removes the default ACL grant.
		err := owner.UpdateMCPServerConfigACL(ctx, firstUser.OrganizationID, config.ID, codersdk.UpdateMCPServerConfigACLRequest{
			GroupRoles: map[string]codersdk.MCPServerConfigRole{
				firstUser.OrganizationID.String(): codersdk.MCPServerConfigRoleDeleted,
			},
		})
		require.NoError(t, err)

		//nolint:gocritic // Owner access isolates custom-role setup from the behavior under test.
		role, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
			Name:           "mcp-update-only-audit",
			OrganizationID: firstUser.OrganizationID.String(),
			OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceMCPServerConfig: {codersdk.ActionUpdate},
				codersdk.ResourceAuditLog:        {codersdk.ActionRead},
			}),
		})
		require.NoError(t, err)
		updateOnly, _ := coderdtest.CreateAnotherUser(t, owner, firstUser.OrganizationID,
			rbac.RoleIdentifier{Name: role.Name, OrganizationID: firstUser.OrganizationID})

		err = owner.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action:         codersdk.AuditActionCreate,
			ResourceType:   codersdk.ResourceTypeMCPServerConfig,
			ResourceID:     config.ID,
			OrganizationID: firstUser.OrganizationID,
		})
		require.NoError(t, err)

		// Update-only administrators lack config read access, yet the
		// settings page admits them, so the link must still appear.
		logs, err := updateOnly.AuditLogs(ctx, codersdk.AuditLogsRequest{
			Pagination: codersdk.Pagination{
				Limit: 1,
			},
		})
		require.NoError(t, err)
		require.Len(t, logs.AuditLogs, 1)
		require.Contains(t, logs.AuditLogs[0].ResourceLink, fmt.Sprintf("/ai/settings/mcp-servers/%s", config.ID))
	})
}
