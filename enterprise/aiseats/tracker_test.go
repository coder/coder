package aiseats_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agplaiseats "github.com/coder/coder/v2/coderd/aiseats"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	enterpriseaiseats "github.com/coder/coder/v2/enterprise/aiseats"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestSeatTrackerDB(t *testing.T) {
	t.Parallel()

	t.Run("ActiveUserRecorded", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		tracker := enterpriseaiseats.New(db, testutil.Logger(t), clock, nil)

		user := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
		tracker.RecordUsage(ctx, user.ID, agplaiseats.ReasonAIBridge("active user event"))

		count, err := db.GetActiveAISeatCount(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
	})

	t.Run("InactiveUsersExcluded", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		tracker := enterpriseaiseats.New(db, testutil.Logger(t), quartz.NewMock(t), nil)

		dormantUser := dbgen.User(t, db, database.User{Status: database.UserStatusDormant})
		tracker.RecordUsage(ctx, dormantUser.ID, agplaiseats.ReasonTask("dormant user event"))

		suspendedUser := dbgen.User(t, db, database.User{Status: database.UserStatusSuspended})
		tracker.RecordUsage(ctx, suspendedUser.ID, agplaiseats.ReasonTask("suspended user event"))

		count, err := db.GetActiveAISeatCount(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 0, count)
	})

	t.Run("StatusTransitions", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		a := audit.NewMock()
		var aI audit.Auditor = a
		var al atomic.Pointer[audit.Auditor]
		al.Store(&aI)

		tracker := enterpriseaiseats.New(db, testutil.Logger(t), quartz.NewMock(t), &al)

		user := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
		tracker.RecordUsage(ctx, user.ID, agplaiseats.ReasonAIBridge("status transition"))

		count, err := db.GetActiveAISeatCount(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 1, count)

		_, err = db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
			ID:         user.ID,
			Status:     database.UserStatusDormant,
			UpdatedAt:  dbtime.Now(),
			UserIsSeen: false,
		})
		require.NoError(t, err)

		count, err = db.GetActiveAISeatCount(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 0, count)

		_, err = db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
			ID:         user.ID,
			Status:     database.UserStatusActive,
			UpdatedAt:  dbtime.Now().Add(time.Second),
			UserIsSeen: false,
		})
		require.NoError(t, err)

		count, err = db.GetActiveAISeatCount(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 1, count)

		require.Len(t, a.AuditLogs(), 1)
		require.Equal(t, database.ResourceTypeAiSeat, a.AuditLogs()[0].ResourceType)
	})
}
