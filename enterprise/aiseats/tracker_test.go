package aiseats_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agplaiseats "github.com/coder/coder/v2/coderd/aiseats"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
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

	// Regression test for coder/internal#1444: UpsertAISeatState must
	// succeed when called through the AsAIBridged RBAC subject. The
	// aibridged daemon context was missing ResourceSystem.ActionCreate,
	// which caused the very first RecordUsage call per user to fail
	// with "unauthorized: rbac: forbidden".
	t.Run("AsAIBridgedRBAC", func(t *testing.T) {
		t.Parallel()

		rawDB, _ := dbtestutil.NewDB(t)
		authz := rbac.NewStrictAuthorizer(prometheus.NewRegistry())
		authzDB := dbauthz.New(rawDB, authz, slogtest.Make(t, nil), coderdtest.AccessControlStorePointer())

		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		tracker := enterpriseaiseats.New(authzDB, testutil.Logger(t), clock, nil)

		// Insert a user directly in the raw DB so it exists for the
		// foreign key reference.
		user := dbgen.User(t, rawDB, database.User{Status: database.UserStatusActive})

		// Call RecordUsage with the AIBridged context, mirroring the
		// production call path in aibridgedserver.RecordInterception.
		aibridgedCtx := dbauthz.AsAIBridged(ctx)
		tracker.RecordUsage(aibridgedCtx, user.ID, agplaiseats.ReasonAIBridge("provider=test, model=test"))

		// Verify the seat was actually recorded. A count of 0 means
		// the upsert was silently rejected by RBAC.
		count, err := rawDB.GetActiveAISeatCount(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 1, count, "AI seat should be recorded when using AsAIBridged context")
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
