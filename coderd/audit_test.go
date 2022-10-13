package coderd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
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

		count, err := client.AuditLogCount(ctx, codersdk.AuditLogCountRequest{})
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

	t.Run("Filter", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		userResourceID := uuid.New()

		// Create two logs with "Create"
		err := client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action:       codersdk.AuditActionCreate,
			ResourceType: codersdk.ResourceTypeTemplate,
		})
		require.NoError(t, err)
		err = client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action:       codersdk.AuditActionCreate,
			ResourceType: codersdk.ResourceTypeUser,
			ResourceID:   userResourceID,
		})
		require.NoError(t, err)

		// Create one log with "Delete"
		err = client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action:       codersdk.AuditActionDelete,
			ResourceType: codersdk.ResourceTypeUser,
			ResourceID:   userResourceID,
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
				ExpectedResult: 3,
			},
			{
				Name:           "FilterByUsername",
				SearchQuery:    "username:" + coderdtest.FirstUserParams.Username,
				ExpectedResult: 3,
			},
			{
				Name:           "FilterByResourceID",
				SearchQuery:    "resource_id:" + userResourceID.String(),
				ExpectedResult: 2,
			},
			{
				Name:           "FilterInvalidSingleValue",
				SearchQuery:    "invalid",
				ExpectedResult: 3,
			},
			{
				Name:           "FilterWithInvalidResourceType",
				SearchQuery:    "resource_type:invalid",
				ExpectedResult: 3,
			},
			{
				Name:           "FilterWithInvalidAction",
				SearchQuery:    "action:invalid",
				ExpectedResult: 3,
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
			})

			// Test count filtering
			t.Run("GetCount"+testCase.Name, func(t *testing.T) {
				t.Parallel()
				response, err := client.AuditLogCount(ctx, codersdk.AuditLogCountRequest{
					SearchQuery: testCase.SearchQuery,
				})
				require.NoError(t, err, "fetch audit logs count")
				require.Equal(t, int(response.Count), testCase.ExpectedResult, "expected audit logs count returned")
			})
		}
	})
}
