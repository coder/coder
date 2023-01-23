package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
)

func TestAuditLogs(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		err := client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			ResourceID: user.UserID,
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
	})
}

func TestAuditLogsFilter(t *testing.T) {
	t.Parallel()

	t.Run("Filter", func(t *testing.T) {
		t.Parallel()

		var (
			ctx      = context.Background()
			client   = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user     = coderdtest.CreateFirstUser(t, client)
			version  = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		)

		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		// Create two logs with "Create"
		err := client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action:       codersdk.AuditActionCreate,
			ResourceType: codersdk.ResourceTypeTemplate,
			ResourceID:   template.ID,
			Time:         time.Date(2022, 8, 15, 14, 30, 45, 100, time.UTC), // 2022-8-15 14:30:45
		})
		require.NoError(t, err)
		err = client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action:       codersdk.AuditActionCreate,
			ResourceType: codersdk.ResourceTypeUser,
			ResourceID:   user.UserID,
			Time:         time.Date(2022, 8, 16, 14, 30, 45, 100, time.UTC), // 2022-8-16 14:30:45
		})
		require.NoError(t, err)

		// Create one log with "Delete"
		err = client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action:       codersdk.AuditActionDelete,
			ResourceType: codersdk.ResourceTypeUser,
			ResourceID:   user.UserID,
			Time:         time.Date(2022, 8, 15, 14, 30, 45, 100, time.UTC), // 2022-8-15 14:30:45
		})
		require.NoError(t, err)

		// Create one log with "Start"
		err = client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action:       codersdk.AuditActionStart,
			ResourceType: codersdk.ResourceTypeWorkspaceBuild,
			ResourceID:   workspace.LatestBuild.ID,
			Time:         time.Date(2022, 8, 15, 14, 30, 45, 100, time.UTC), // 2022-8-15 14:30:45
		})
		require.NoError(t, err)

		// Create one log with "Stop"
		err = client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action:       codersdk.AuditActionStop,
			ResourceType: codersdk.ResourceTypeWorkspaceBuild,
			ResourceID:   workspace.LatestBuild.ID,
			Time:         time.Date(2022, 8, 15, 14, 30, 45, 100, time.UTC), // 2022-8-15 14:30:45
		})
		require.NoError(t, err)

		// Test cases
		testCases := []struct {
			Name           string
			SearchQuery    string
			ExpectedResult int
		}{
			{
				Name:           "FilterByCreateAction",
				SearchQuery:    "action:create",
				ExpectedResult: 2,
			},
			{
				Name:           "FilterByDeleteAction",
				SearchQuery:    "action:delete",
				ExpectedResult: 1,
			},
			{
				Name:           "FilterByUserResourceType",
				SearchQuery:    "resource_type:user",
				ExpectedResult: 2,
			},
			{
				Name:           "FilterByTemplateResourceType",
				SearchQuery:    "resource_type:template",
				ExpectedResult: 1,
			},
			{
				Name:           "FilterByEmail",
				SearchQuery:    "email:" + coderdtest.FirstUserParams.Email,
				ExpectedResult: 5,
			},
			{
				Name:           "FilterByUsername",
				SearchQuery:    "username:" + coderdtest.FirstUserParams.Username,
				ExpectedResult: 5,
			},
			{
				Name:           "FilterByResourceID",
				SearchQuery:    "resource_id:" + user.UserID.String(),
				ExpectedResult: 2,
			},
			{
				Name:           "FilterInvalidSingleValue",
				SearchQuery:    "invalid",
				ExpectedResult: 5,
			},
			{
				Name:           "FilterWithInvalidResourceType",
				SearchQuery:    "resource_type:invalid",
				ExpectedResult: 5,
			},
			{
				Name:           "FilterWithInvalidAction",
				SearchQuery:    "action:invalid",
				ExpectedResult: 5,
			},
			{
				Name:           "FilterOnCreateSingleDay",
				SearchQuery:    "action:create date_from:2022-08-15 date_to:2022-08-15",
				ExpectedResult: 1,
			},
			{
				Name:           "FilterOnCreateDateFrom",
				SearchQuery:    "action:create date_from:2022-08-15",
				ExpectedResult: 2,
			},
			{
				Name:           "FilterOnCreateDateTo",
				SearchQuery:    "action:create date_to:2022-08-15",
				ExpectedResult: 1,
			},
			{
				Name:           "FilterOnWorkspaceBuildStart",
				SearchQuery:    "resource_type:workspace_build action:start",
				ExpectedResult: 1,
			},
			{
				Name:           "FilterOnWorkspaceBuildStop",
				SearchQuery:    "resource_type:workspace_build action:stop",
				ExpectedResult: 1,
			},
		}

		for _, testCase := range testCases {
			testCase := testCase
			// Test filtering
			t.Run(testCase.Name, func(t *testing.T) {
				t.Parallel()
				auditLogs, err := client.AuditLogs(ctx, codersdk.AuditLogsRequest{
					SearchQuery: testCase.SearchQuery,
					Pagination: codersdk.Pagination{
						Limit: 25,
					},
				})
				require.NoError(t, err, "fetch audit logs")
				require.Len(t, auditLogs.AuditLogs, testCase.ExpectedResult, "expected audit logs returned")
				require.Equal(t, testCase.ExpectedResult, int(auditLogs.Count), "expected audit log count returned")
			})
		}
	})
}
