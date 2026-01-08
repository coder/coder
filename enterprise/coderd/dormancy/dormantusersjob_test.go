package dormancy_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/coderd/dormancy"
	"github.com/coder/quartz"
)

func TestCheckInactiveUsers(t *testing.T) {
	t.Parallel()

	// Predefine job settings
	interval := time.Millisecond
	dormancyPeriod := 90 * 24 * time.Hour

	// Add some dormant accounts
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	db, _ := dbtestutil.NewDB(t)

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	// Use a fixed base time to avoid timing races
	baseTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	dormancyThreshold := baseTime.Add(-dormancyPeriod)

	// Create inactive users (last seen BEFORE dormancy threshold)
	inactiveUser1 := setupUser(ctx, t, db, "dormant-user-1@coder.com", database.UserStatusActive, dormancyThreshold.Add(-time.Minute))
	inactiveUser2 := setupUser(ctx, t, db, "dormant-user-2@coder.com", database.UserStatusActive, dormancyThreshold.Add(-time.Hour))
	inactiveUser3 := setupUser(ctx, t, db, "dormant-user-3@coder.com", database.UserStatusActive, dormancyThreshold.Add(-6*time.Hour))

	// Create active users (last seen AFTER dormancy threshold)
	activeUser1 := setupUser(ctx, t, db, "active-user-1@coder.com", database.UserStatusActive, baseTime.Add(-time.Minute))
	activeUser2 := setupUser(ctx, t, db, "active-user-2@coder.com", database.UserStatusActive, baseTime.Add(-time.Hour))
	activeUser3 := setupUser(ctx, t, db, "active-user-3@coder.com", database.UserStatusActive, baseTime.Add(-6*time.Hour))

	suspendedUser1 := setupUser(ctx, t, db, "suspended-user-1@coder.com", database.UserStatusSuspended, dormancyThreshold.Add(-time.Minute))
	suspendedUser2 := setupUser(ctx, t, db, "suspended-user-2@coder.com", database.UserStatusSuspended, dormancyThreshold.Add(-time.Hour))
	suspendedUser3 := setupUser(ctx, t, db, "suspended-user-3@coder.com", database.UserStatusSuspended, dormancyThreshold.Add(-6*time.Hour))

	mAudit := audit.NewMock()
	mClock := quartz.NewMock(t)
	// Set the mock clock to the base time to ensure consistent behavior
	mClock.Set(baseTime)
	// Run the periodic job
	closeFunc := dormancy.CheckInactiveUsersWithOptions(ctx, logger, mClock, db, mAudit, interval, dormancyPeriod)
	t.Cleanup(closeFunc)

	dur, w := mClock.AdvanceNext()
	require.Equal(t, interval, dur)
	w.MustWait(ctx)

	rows, err := db.GetUsers(ctx, database.GetUsersParams{})
	require.NoError(t, err)

	var dormant, suspended int
	for _, row := range rows {
		if row.Status == database.UserStatusDormant {
			dormant++
		} else if row.Status == database.UserStatusSuspended {
			suspended++
		}
	}

	// 9 users in total, 3 active, 3 dormant, 3 suspended
	require.Len(t, rows, 9)
	require.Equal(t, 3, dormant)
	require.Equal(t, 3, suspended)

	require.Len(t, mAudit.AuditLogs(), 3)

	allUsers := ignoreUpdatedAt(database.ConvertUserRows(rows))

	// Verify user status
	expectedUsers := ignoreUpdatedAt([]database.User{
		asDormant(inactiveUser1),
		asDormant(inactiveUser2),
		asDormant(inactiveUser3),
		activeUser1,
		activeUser2,
		activeUser3,
		suspendedUser1,
		suspendedUser2,
		suspendedUser3,
	})

	require.ElementsMatch(t, allUsers, expectedUsers)
}

func setupUser(ctx context.Context, t *testing.T, db database.Store, email string, status database.UserStatus, lastSeenAt time.Time) database.User {
	t.Helper()

	now := dbtestutil.NowInDefaultTimezone()
	user, err := db.InsertUser(ctx, database.InsertUserParams{
		ID:        uuid.New(),
		LoginType: database.LoginTypePassword,
		Username:  uuid.NewString()[:8],
		Email:     email,
		RBACRoles: []string{},
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)
	// At the beginning of the test all users are marked as active
	user, err = db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{ID: user.ID, Status: status})
	require.NoError(t, err)
	user, err = db.UpdateUserLastSeenAt(ctx, database.UpdateUserLastSeenAtParams{ID: user.ID, LastSeenAt: lastSeenAt})
	require.NoError(t, err)
	return user
}

func asDormant(user database.User) database.User {
	user.Status = database.UserStatusDormant
	return user
}

func ignoreUpdatedAt(rows []database.User) []database.User {
	for i := range rows {
		rows[i].UpdatedAt = time.Time{}
	}
	return rows
}
