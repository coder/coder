package coderd_test

import (
	"context"
	"testing"

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
		_ = coderdtest.CreateFirstUser(t, client)

		err := client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{})
		require.NoError(t, err)

		count, err := client.AuditLogCount(ctx)
		require.NoError(t, err)

		alogs, err := client.AuditLogs(ctx, codersdk.AuditLogsRequest{
			Pagination: codersdk.Pagination{
				Limit: 1,
			},
		})
		require.NoError(t, err)

		require.Equal(t, int64(1), count.Count)
		require.Len(t, alogs.AuditLogs, 1)
	})
}

func TestAuditLogsFilter(t *testing.T) {
	t.Parallel()

	t.Run("FilterByResourceType", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		// Create two logs with "Create"
		err := client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action:       codersdk.AuditActionCreate,
			ResourceType: codersdk.ResourceTypeTemplate,
		})
		require.NoError(t, err)
		err = client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action:       codersdk.AuditActionCreate,
			ResourceType: codersdk.ResourceTypeUser,
		})
		require.NoError(t, err)

		// Create one log with "Delete"
		err = client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action:       codersdk.AuditActionDelete,
			ResourceType: codersdk.ResourceTypeUser,
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
		}

		for _, testCase := range testCases {
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
			})
		}
	})
}
