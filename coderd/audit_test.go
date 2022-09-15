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
			Action: codersdk.AuditActionCreate,
		})
		require.NoError(t, err)
		err = client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action: codersdk.AuditActionCreate,
		})
		require.NoError(t, err)
		// Create one log with "Delete"
		err = client.CreateTestAuditLog(ctx, codersdk.CreateTestAuditLogRequest{
			Action: codersdk.AuditActionDelete,
		})
		require.NoError(t, err)

		// Verify the number of create logs
		actionCreateLogs, err := client.AuditLogs(ctx, codersdk.AuditLogsRequest{
			SearchQuery: "action:create",
		})
		require.NoError(t, err)
		require.Len(t, actionCreateLogs, 2)

		// Verify the number of delete logs
		actionDeleteLogs, err := client.AuditLogs(ctx, codersdk.AuditLogsRequest{
			SearchQuery: "action:delete",
		})
		require.NoError(t, err)
		require.Len(t, actionDeleteLogs, 1)
	})
}
