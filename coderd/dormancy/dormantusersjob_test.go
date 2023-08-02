package dormancy_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/dormancy"
	"github.com/coder/coder/testutil"
)

func TestCheckInactiveUsers(t *testing.T) {
	t.Parallel()

	// Predefine job settings
	interval := time.Millisecond
	dormancyPeriod := 90 * 24 * time.Hour

	// Add some dormant accounts
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	db := dbfake.New()

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	inactiveUser1 := setupUser(ctx, t, db, "dormant-user-1@coder.com", time.Now().Add(-dormancyPeriod).Add(-time.Minute))
	inactiveUser2 := setupUser(ctx, t, db, "dormant-user-2@coder.com", time.Now().Add(-dormancyPeriod).Add(-time.Hour))
	inactiveUser3 := setupUser(ctx, t, db, "dormant-user-3@coder.com", time.Now().Add(-dormancyPeriod).Add(-6*time.Hour))

	activeUser1 := setupUser(ctx, t, db, "active-user-1@coder.com", time.Now().Add(-dormancyPeriod).Add(time.Minute))
	activeUser2 := setupUser(ctx, t, db, "active-user-2@coder.com", time.Now().Add(-dormancyPeriod).Add(time.Hour))
	activeUser3 := setupUser(ctx, t, db, "active-user-3@coder.com", time.Now().Add(-dormancyPeriod).Add(6*time.Hour))

	// Run the periodic job
	closeFunc := dormancy.CheckInactiveUsersWithOptions(ctx, logger, db, interval, dormancyPeriod)
	t.Cleanup(closeFunc)

	var rows []database.GetUsersRow
	var err error
	require.Eventually(t, func() bool {
		rows, err = db.GetUsers(ctx, database.GetUsersParams{})
		if err != nil {
			return false
		}

		var c int
		for _, row := range rows {
			if row.Status == database.UserStatusDormant {
				c++
			}
		}
		// 6 users in total, 3 dormant
		return len(rows) == 6 && c == 3
	}, testutil.WaitShort, testutil.IntervalMedium)

	allUsers := database.ConvertUserRows(rows)

	// Verify user status
	expectedUsers := []database.User{
		asDormant(inactiveUser1),
		asDormant(inactiveUser2),
		asDormant(inactiveUser3),
		activeUser1,
		activeUser2,
		activeUser3,
	}
	require.ElementsMatch(t, allUsers, expectedUsers)
}

func setupUser(ctx context.Context, t *testing.T, db database.Store, email string, lastSeenAt time.Time) database.User {
	t.Helper()

	user, err := db.InsertUser(ctx, database.InsertUserParams{ID: uuid.New(), LoginType: database.LoginTypePassword, Username: namesgenerator.GetRandomName(8), Email: email})
	require.NoError(t, err)
	// At the beginning of the test all users are marked as active
	user, err = db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{ID: user.ID, Status: database.UserStatusActive})
	require.NoError(t, err)
	user, err = db.UpdateUserLastSeenAt(ctx, database.UpdateUserLastSeenAtParams{ID: user.ID, LastSeenAt: lastSeenAt})
	require.NoError(t, err)
	return user
}

func asDormant(user database.User) database.User {
	user.Status = database.UserStatusDormant
	return user
}
