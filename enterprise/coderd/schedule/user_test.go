package schedule_test

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	agpl "github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/enterprise/coderd/schedule"
)

func TestEnterpriseUserQuietHoursSchedule(t *testing.T) {
	t.Parallel()

	const (
		defaultSchedule     = "CRON_TZ=UTC 15 10 * * *"
		userCustomSchedule1 = "CRON_TZ=Australia/Sydney 30 2 * * *"
		userCustomSchedule2 = "CRON_TZ=Australia/Sydney 0 18 * * *"
	)

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		s, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(defaultSchedule, true)
		require.NoError(t, err)

		mDB := dbmock.NewMockStore(gomock.NewController(t))

		// User has no schedule set, use default.
		mDB.EXPECT().GetUserByID(gomock.Any(), userID).Return(database.User{}, nil).Times(1)
		opts, err := s.Get(context.Background(), mDB, userID)
		require.NoError(t, err)
		require.NotNil(t, opts.Schedule)
		require.Equal(t, defaultSchedule, opts.Schedule.String())
		require.False(t, opts.UserSet)
		require.True(t, opts.UserCanSet)

		// User has a custom schedule set.
		mDB.EXPECT().GetUserByID(gomock.Any(), userID).Return(database.User{
			QuietHoursSchedule: userCustomSchedule1,
		}, nil).Times(1)
		opts, err = s.Get(context.Background(), mDB, userID)
		require.NoError(t, err)
		require.NotNil(t, opts.Schedule)
		require.Equal(t, userCustomSchedule1, opts.Schedule.String())
		require.True(t, opts.UserSet)
		require.True(t, opts.UserCanSet)

		// Set user schedule.
		mDB.EXPECT().UpdateUserQuietHoursSchedule(gomock.Any(), database.UpdateUserQuietHoursScheduleParams{
			ID:                 userID,
			QuietHoursSchedule: userCustomSchedule2,
		}).Return(database.User{}, nil).Times(1)
		opts, err = s.Set(context.Background(), mDB, userID, userCustomSchedule2)
		require.NoError(t, err)
		require.NotNil(t, opts.Schedule)
		require.Equal(t, userCustomSchedule2, opts.Schedule.String())
		require.True(t, opts.UserSet)
	})

	t.Run("BadDefaultSchedule", func(t *testing.T) {
		t.Parallel()

		_, err := schedule.NewEnterpriseUserQuietHoursScheduleStore("bad schedule", true)
		require.Error(t, err)
		require.ErrorContains(t, err, `parse daily schedule "bad schedule"`)
	})

	t.Run("BadGotSchedule", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		s, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(defaultSchedule, true)
		require.NoError(t, err)

		mDB := dbmock.NewMockStore(gomock.NewController(t))

		// User has a custom schedule set.
		mDB.EXPECT().GetUserByID(gomock.Any(), userID).Return(database.User{
			QuietHoursSchedule: "bad schedule",
		}, nil).Times(1)
		_, err = s.Get(context.Background(), mDB, userID)
		require.Error(t, err)
		require.ErrorContains(t, err, `parse daily schedule "bad schedule"`)
	})

	t.Run("BadSetSchedule", func(t *testing.T) {
		t.Parallel()

		s, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(defaultSchedule, true)
		require.NoError(t, err)

		// Use the mock DB here. It won't get used, but if it ever does it will
		// fail the test.
		mDB := dbmock.NewMockStore(gomock.NewController(t))
		_, err = s.Set(context.Background(), mDB, uuid.New(), "bad schedule")
		require.Error(t, err)
		require.ErrorContains(t, err, `parse daily schedule "bad schedule"`)
	})

	t.Run("UserCannotSet", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		s, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(defaultSchedule, false) // <---
		require.NoError(t, err)

		// Use the mock DB here. It won't get used, but if it ever does it will
		// fail the test.
		mDB := dbmock.NewMockStore(gomock.NewController(t))

		// Should never reach out to DB to check user's custom schedule.
		opts, err := s.Get(context.Background(), mDB, userID)
		require.NoError(t, err)
		require.NotNil(t, opts.Schedule)
		require.Equal(t, defaultSchedule, opts.Schedule.String())
		require.False(t, opts.UserSet)
		require.False(t, opts.UserCanSet)

		// Set user schedule should fail.
		_, err = s.Set(context.Background(), mDB, userID, userCustomSchedule1)
		require.Error(t, err)
		require.ErrorIs(t, err, agpl.ErrUserCannotSetQuietHoursSchedule)
	})
}
