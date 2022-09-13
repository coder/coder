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

		err := client.CreateTestAuditLog(ctx)
		require.NoError(t, err)

		count, err := client.AuditLogCount(ctx)
		require.NoError(t, err)

		alogs, err := client.AuditLogs(ctx, codersdk.Pagination{Limit: 1})
		require.NoError(t, err)

		require.Equal(t, int64(1), count.Count)
		require.Len(t, alogs.AuditLogs, 1)
	})
}
