package database_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestUpsertAISeatState(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	ctx := testutil.Context(t, testutil.WaitShort)

	t1 := dbtime.Now().Add(-time.Minute)
	err := db.UpsertAISeatState(ctx, database.UpsertAISeatStateParams{
		UserID:               user.ID,
		FirstUsedAt:          t1,
		LastEventType:        database.AiSeatUsageReasonAibridge,
		LastEventDescription: "first",
	})
	require.NoError(t, err)

	t2 := t1.Add(time.Hour)
	err = db.UpsertAISeatState(ctx, database.UpsertAISeatStateParams{
		UserID:               user.ID,
		FirstUsedAt:          t2,
		LastEventType:        database.AiSeatUsageReasonTask,
		LastEventDescription: "second",
	})
	require.NoError(t, err)

	rows, err := db.ListAISeatState(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, user.ID, rows[0].UserID)
	require.True(t, t1.Equal(rows[0].FirstUsedAt))
	require.True(t, t1.Equal(rows[0].LastUsedAt))
	require.Equal(t, database.AiSeatUsageReasonAibridge, rows[0].LastEventType)
	require.Equal(t, "first", rows[0].LastEventDescription)

}

func TestGetActiveAISeatCount(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := dbtime.Now()

	active := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
	dormant := dbgen.User(t, db, database.User{Status: database.UserStatusDormant})

	err := db.UpsertAISeatState(ctx, database.UpsertAISeatStateParams{
		UserID:               active.ID,
		FirstUsedAt:          now,
		LastEventType:        database.AiSeatUsageReasonAibridge,
		LastEventDescription: "active",
	})
	require.NoError(t, err)

	err = db.UpsertAISeatState(ctx, database.UpsertAISeatStateParams{
		UserID:               dormant.ID,
		FirstUsedAt:          now,
		LastEventType:        database.AiSeatUsageReasonTask,
		LastEventDescription: "dormant",
	})
	require.NoError(t, err)

	count, err := db.GetActiveAISeatCount(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}
