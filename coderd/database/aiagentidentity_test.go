package database_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestAuditLogsDelegatedUserFilters(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	ownerID := uuid.New()
	agentID := uuid.New()
	dbgen.AuditLog(t, db, database.AuditLog{UserID: ownerID})
	dbgen.AuditLog(t, db, database.AuditLog{
		UserID: agentID,
		OnBehalfOfUserID: uuid.NullUUID{
			UUID:  ownerID,
			Valid: true,
		},
	})

	principalLogs, err := db.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{
		UserID: ownerID,
	})
	require.NoError(t, err)
	require.Len(t, principalLogs, 2)

	delegatedLogs, err := db.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{
		OnBehalfOfUserID: ownerID,
	})
	require.NoError(t, err)
	require.Len(t, delegatedLogs, 1)
	require.Equal(t, agentID, delegatedLogs[0].AuditLog.UserID)
}
