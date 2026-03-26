package cronscheduler_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/automations"
	"github.com/coder/coder/v2/coderd/automations/cronscheduler"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

// awaitDoTick waits for the scheduler to complete its initial tick.
// It traps the quartz.Mock clock events in the same order the
// scheduler emits them: Now() → TickerReset.
func awaitDoTick(ctx context.Context, t *testing.T, clk *quartz.Mock) chan struct{} {
	t.Helper()
	ch := make(chan struct{})
	trapNow := clk.Trap().Now()
	trapReset := clk.Trap().TickerReset()
	go func() {
		defer close(ch)
		defer trapReset.Close()
		defer trapNow.Close()
		// Wait for the initial Now() call that kicks off doTick.
		trapNow.MustWait(ctx).MustRelease(ctx)
		// Wait for the ticker reset that signals doTick completed.
		trapReset.MustWait(ctx).MustRelease(ctx)
	}()
	return ch
}

// fakeChatCreator is a stub ChatCreator for tests. CreateChat
// returns a new UUID; SendMessage is a no-op.
type fakeChatCreator struct{}

func (fakeChatCreator) CreateChat(_ context.Context, _ automations.CreateChatOptions) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (fakeChatCreator) SendMessage(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ string) error {
	return nil
}

// makeTrigger builds a GetActiveCronTriggersRow with sensible
// defaults. Fields can be overridden after creation.
func makeTrigger(schedule string, status string, createdAt time.Time) database.GetActiveCronTriggersRow {
	return database.GetActiveCronTriggersRow{
		ID:                              uuid.New(),
		AutomationID:                    uuid.New(),
		Type:                            "cron",
		CronSchedule:                    sql.NullString{String: schedule, Valid: true},
		CreatedAt:                       createdAt,
		UpdatedAt:                       createdAt,
		AutomationStatus:                status,
		AutomationOwnerID:               uuid.New(),
		AutomationMaxChatCreatesPerHour: 10,
		AutomationMaxMessagesPerHour:    100,
	}
}

//nolint:paralleltest // Uses LockIDAutomationCron advisory lock mock.
func TestScheduler(t *testing.T) {
	t.Run("NoTriggers", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		clk := quartz.NewMock(t)

		// The scheduler calls InTx; execute the function against
		// the same mock so inner calls are recorded.
		mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(fn func(database.Store) error, _ *database.TxOptions) error {
				return fn(mDB)
			},
		).Times(1)
		mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDAutomationCron)).Return(true, nil)
		mDB.EXPECT().GetActiveCronTriggers(gomock.Any()).Return(nil, nil)

		done := awaitDoTick(ctx, t, clk)
		scheduler := cronscheduler.New(ctx, testutil.Logger(t), mDB, clk, nil)
		<-done
		require.NoError(t, scheduler.Close())
	})

	t.Run("DueTriggerFires", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		clk := quartz.NewMock(t)

		now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
		clk.Set(now).MustWait(ctx)

		// Trigger was created 2 minutes ago with "every minute"
		// schedule, so it should fire.
		trigger := makeTrigger("* * * * *", "active", now.Add(-2*time.Minute))

		mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(fn func(database.Store) error, _ *database.TxOptions) error {
				return fn(mDB)
			},
		)
		mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDAutomationCron)).Return(true, nil)
		mDB.EXPECT().GetActiveCronTriggers(gomock.Any()).Return(
			[]database.GetActiveCronTriggersRow{trigger}, nil,
		)

		// Fire() checks rate limits before creating a chat.
		mDB.EXPECT().CountAutomationChatCreatesInWindow(gomock.Any(), gomock.Any()).Return(int64(0), nil)
		mDB.EXPECT().CountAutomationMessagesInWindow(gomock.Any(), gomock.Any()).Return(int64(0), nil)

		// Expect the event to be inserted with status "created".
		mDB.EXPECT().InsertAutomationEvent(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, arg database.InsertAutomationEventParams) (database.AutomationEvent, error) {
				assert.Equal(t, trigger.AutomationID, arg.AutomationID)
				assert.Equal(t, trigger.ID, arg.TriggerID.UUID)
				assert.Equal(t, "created", arg.Status)
				assert.True(t, arg.FilterMatched)
				return database.AutomationEvent{ID: uuid.New()}, nil
			},
		)
		// Expect last_triggered_at to be updated.
		mDB.EXPECT().UpdateAutomationTriggerLastTriggeredAt(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, arg database.UpdateAutomationTriggerLastTriggeredAtParams) error {
				assert.Equal(t, trigger.ID, arg.ID)
				assert.Equal(t, now, arg.LastTriggeredAt)
				return nil
			},
		)

		done := awaitDoTick(ctx, t, clk)
		scheduler := cronscheduler.New(ctx, testutil.Logger(t), mDB, clk, fakeChatCreator{})
		<-done
		require.NoError(t, scheduler.Close())
	})

	t.Run("NotYetDueTriggerSkipped", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		clk := quartz.NewMock(t)

		now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
		clk.Set(now).MustWait(ctx)

		// Trigger created now with "every hour" schedule. Next fire
		// is 1 hour from now, so it should NOT fire.
		trigger := makeTrigger("0 * * * *", "active", now)

		mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(fn func(database.Store) error, _ *database.TxOptions) error {
				return fn(mDB)
			},
		)
		mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDAutomationCron)).Return(true, nil)
		mDB.EXPECT().GetActiveCronTriggers(gomock.Any()).Return(
			[]database.GetActiveCronTriggersRow{trigger}, nil,
		)
		// No InsertAutomationEvent or UpdateAutomationTriggerLastTriggeredAt
		// expected — the trigger is not due.

		done := awaitDoTick(ctx, t, clk)
		scheduler := cronscheduler.New(ctx, testutil.Logger(t), mDB, clk, nil)
		<-done
		require.NoError(t, scheduler.Close())
	})

	t.Run("PreviewModeCreatesPreviewEvent", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		clk := quartz.NewMock(t)

		now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
		clk.Set(now).MustWait(ctx)

		trigger := makeTrigger("* * * * *", "preview", now.Add(-2*time.Minute))

		mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(fn func(database.Store) error, _ *database.TxOptions) error {
				return fn(mDB)
			},
		)
		mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDAutomationCron)).Return(true, nil)
		mDB.EXPECT().GetActiveCronTriggers(gomock.Any()).Return(
			[]database.GetActiveCronTriggersRow{trigger}, nil,
		)
		mDB.EXPECT().InsertAutomationEvent(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, arg database.InsertAutomationEventParams) (database.AutomationEvent, error) {
				assert.Equal(t, "preview", arg.Status)
				return database.AutomationEvent{ID: uuid.New()}, nil
			},
		)
		mDB.EXPECT().UpdateAutomationTriggerLastTriggeredAt(gomock.Any(), gomock.Any()).Return(nil)

		done := awaitDoTick(ctx, t, clk)
		scheduler := cronscheduler.New(ctx, testutil.Logger(t), mDB, clk, nil)
		<-done
		require.NoError(t, scheduler.Close())
	})

	t.Run("InvalidScheduleSkipped", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		clk := quartz.NewMock(t)

		now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
		clk.Set(now).MustWait(ctx)

		trigger := makeTrigger("not a cron", "active", now.Add(-2*time.Minute))

		mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(fn func(database.Store) error, _ *database.TxOptions) error {
				return fn(mDB)
			},
		)
		mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDAutomationCron)).Return(true, nil)
		mDB.EXPECT().GetActiveCronTriggers(gomock.Any()).Return(
			[]database.GetActiveCronTriggersRow{trigger}, nil,
		)
		// No event insert expected — invalid schedule is skipped.

		done := awaitDoTick(ctx, t, clk)
		scheduler := cronscheduler.New(ctx, testutil.Logger(t), mDB, clk, nil)
		<-done
		require.NoError(t, scheduler.Close())
	})

	t.Run("LastTriggeredAtPreventsRefire", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		clk := quartz.NewMock(t)

		now := time.Date(2025, 6, 15, 12, 0, 30, 0, time.UTC)
		clk.Set(now).MustWait(ctx)

		// Trigger with "every hour" schedule that last fired at the
		// top of this hour. Next fire is 13:00, which is after now.
		trigger := makeTrigger("0 * * * *", "active", now.Add(-24*time.Hour))
		trigger.LastTriggeredAt = sql.NullTime{
			Time:  time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
			Valid: true,
		}

		mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(fn func(database.Store) error, _ *database.TxOptions) error {
				return fn(mDB)
			},
		)
		mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDAutomationCron)).Return(true, nil)
		mDB.EXPECT().GetActiveCronTriggers(gomock.Any()).Return(
			[]database.GetActiveCronTriggersRow{trigger}, nil,
		)
		// No event insert — last_triggered_at means next fire is
		// in the future.

		done := awaitDoTick(ctx, t, clk)
		scheduler := cronscheduler.New(ctx, testutil.Logger(t), mDB, clk, nil)
		<-done
		require.NoError(t, scheduler.Close())
	})

	t.Run("LockNotAcquiredSkips", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		clk := quartz.NewMock(t)

		mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(fn func(database.Store) error, _ *database.TxOptions) error {
				return fn(mDB)
			},
		)
		// Another replica holds the lock.
		mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDAutomationCron)).Return(false, nil)
		// No GetActiveCronTriggers call expected.

		done := awaitDoTick(ctx, t, clk)
		scheduler := cronscheduler.New(ctx, testutil.Logger(t), mDB, clk, nil)
		<-done
		require.NoError(t, scheduler.Close())
	})
}
