package aiseats_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
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

		rows, err := db.ListAISeatState(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, user.ID, rows[0].UserID)
		require.Equal(t, database.AiSeatUsageReasonAibridge, rows[0].LastEventType)
		require.Equal(t, "active user event", rows[0].LastEventDescription)
	})

	t.Run("DormantUserExcluded", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		tracker := enterpriseaiseats.New(db, testutil.Logger(t), quartz.NewMock(t), nil)

		user := dbgen.User(t, db, database.User{Status: database.UserStatusDormant})
		tracker.RecordUsage(ctx, user.ID, agplaiseats.ReasonTask("dormant user event"))

		count, err := db.GetActiveAISeatCount(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 0, count)

		rows, err := db.ListAISeatState(ctx)
		require.NoError(t, err)
		require.Empty(t, rows)
	})

	t.Run("SuspendedUserExcluded", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		tracker := enterpriseaiseats.New(db, testutil.Logger(t), quartz.NewMock(t), nil)

		user := dbgen.User(t, db, database.User{Status: database.UserStatusSuspended})
		tracker.RecordUsage(ctx, user.ID, agplaiseats.ReasonTask("suspended user event"))

		count, err := db.GetActiveAISeatCount(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 0, count)

		rows, err := db.ListAISeatState(ctx)
		require.NoError(t, err)
		require.Empty(t, rows)
	})

	t.Run("StatusTransitions", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		tracker := enterpriseaiseats.New(db, testutil.Logger(t), quartz.NewMock(t), nil)

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

		rows, err := db.ListAISeatState(ctx)
		require.NoError(t, err)
		require.Empty(t, rows)

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

		rows, err = db.ListAISeatState(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, user.ID, rows[0].UserID)
	})

	t.Run("MultipleActiveUsers", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		tracker := enterpriseaiseats.New(db, testutil.Logger(t), clock, nil)

		user1 := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
		user2 := dbgen.User(t, db, database.User{Status: database.UserStatusActive})

		tracker.RecordUsage(ctx, user1.ID, agplaiseats.ReasonTask("first active user"))
		_ = clock.Advance(time.Second)
		tracker.RecordUsage(ctx, user2.ID, agplaiseats.ReasonTask("second active user"))

		count, err := db.GetActiveAISeatCount(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 2, count)

		rows, err := db.ListAISeatState(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 2)
		require.Equal(t, user2.ID, rows[0].UserID)
		require.Equal(t, user1.ID, rows[1].UserID)
		require.True(t, rows[0].LastUsedAt.After(rows[1].LastUsedAt))
	})

	t.Run("MixedStatuses", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		tracker := enterpriseaiseats.New(db, testutil.Logger(t), quartz.NewMock(t), nil)

		active := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
		dormant := dbgen.User(t, db, database.User{Status: database.UserStatusDormant})
		suspended := dbgen.User(t, db, database.User{Status: database.UserStatusSuspended})

		tracker.RecordUsage(ctx, active.ID, agplaiseats.ReasonAIBridge("active"))
		tracker.RecordUsage(ctx, dormant.ID, agplaiseats.ReasonAIBridge("dormant"))
		tracker.RecordUsage(ctx, suspended.ID, agplaiseats.ReasonTask("suspended"))

		count, err := db.GetActiveAISeatCount(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 1, count)

		rows, err := db.ListAISeatState(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, active.ID, rows[0].UserID)
	})

	t.Run("ReasonTypes", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		tracker := enterpriseaiseats.New(db, testutil.Logger(t), clock, nil)

		aiBridgeUser := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
		taskUser := dbgen.User(t, db, database.User{Status: database.UserStatusActive})

		tracker.RecordUsage(ctx, aiBridgeUser.ID, agplaiseats.ReasonAIBridge("aibridge event"))
		_ = clock.Advance(time.Second)
		tracker.RecordUsage(ctx, taskUser.ID, agplaiseats.ReasonTask("task event"))

		rows, err := db.ListAISeatState(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 2)

		reasonsByUser := map[uuid.UUID]database.AiSeatUsageReason{}
		for _, row := range rows {
			reasonsByUser[row.UserID] = row.LastEventType
		}

		require.Equal(t, database.AiSeatUsageReasonAibridge, reasonsByUser[aiBridgeUser.ID])
		require.Equal(t, database.AiSeatUsageReasonTask, reasonsByUser[taskUser.ID])
	})

	t.Run("FirstUseCreatesAuditLog", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		mockAuditor := audit.NewMock()
		var auditorPtr atomic.Pointer[audit.Auditor]
		var auditor audit.Auditor = mockAuditor
		auditorPtr.Store(&auditor)
		tracker := enterpriseaiseats.New(db, testutil.Logger(t), clock, &auditorPtr)

		user := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
		tracker.RecordUsage(ctx, user.ID, agplaiseats.ReasonAIBridge("first use"))

		clock.Advance(time.Hour * 72)
		tracker.RecordUsage(ctx, user.ID, agplaiseats.ReasonAIBridge("second should write to db, but not create audit log"))

		logs := mockAuditor.AuditLogs()
		require.Len(t, logs, 1)
		require.Equal(t, database.ResourceTypeAiSeat, logs[0].ResourceType)
		require.Equal(t, database.AuditActionCreate, logs[0].Action)
		require.Equal(t, user.ID, logs[0].UserID)
		require.Equal(t, user.ID, logs[0].ResourceID)
	})
}
