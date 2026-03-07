package usage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
	"github.com/coder/coder/v2/enterprise/coderd/usage"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestCron(t *testing.T) {
	t.Parallel()

	t.Run("BasicTick", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		inserted := make(chan string, 1)
		db.EXPECT().InsertUsageEvent(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, params database.InsertUsageEventParams) error {
				inserted <- params.ID
				return nil
			}).AnyTimes()

		inserter := usage.NewDBInserter(usage.InserterWithClock(clock))
		cron := usage.NewCron(clock, slogtest.Make(t, nil), db, inserter)
		require.NoError(t, cron.Register(usage.CronJob{
			Name:     "test-job",
			Interval: 5 * time.Minute,
			Fn: func(_ context.Context) (string, usagetypes.HeartbeatEvent, error) {
				return "test-heartbeat-1", usagetypes.HBAISeats{Count: 42}, nil
			},
		}))

		timerTrap := clock.Trap().NewTimer("test-job")

		cron.Start(ctx)
		defer cron.Close()
		defer timerTrap.Close()

		// Wait for timer creation, verify duration, then fire it.
		timerCall := timerTrap.MustWait(ctx)
		require.Equal(t, 5*time.Minute, timerCall.Duration)
		timerCall.MustRelease(ctx)
		clock.Advance(5 * time.Minute)

		// Verify the event was inserted.
		select {
		case id := <-inserted:
			assert.Equal(t, "test-heartbeat-1", id)
		case <-ctx.Done():
			t.Fatal("timed out waiting for insert")
		}
	})

	t.Run("JitterRange", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		db.EXPECT().InsertUsageEvent(gomock.Any(), gomock.Any()).
			Return(nil).AnyTimes()

		inserter := usage.NewDBInserter(usage.InserterWithClock(clock))
		cron := usage.NewCron(clock, slogtest.Make(t, nil), db, inserter)
		require.NoError(t, cron.Register(usage.CronJob{
			Name:     "jitter-job",
			Interval: 10 * time.Minute,
			Jitter:   2 * time.Minute,
			Fn: func(_ context.Context) (string, usagetypes.HeartbeatEvent, error) {
				return "jitter-hb", usagetypes.HBAISeats{Count: 1}, nil
			},
		}))

		timerTrap := clock.Trap().NewTimer("jitter-job")

		cron.Start(ctx)
		defer cron.Close()
		defer timerTrap.Close()

		// Check several ticks to verify jitter is within bounds.
		minDur := 8 * time.Minute  // interval - jitter
		maxDur := 12 * time.Minute // interval + jitter
		for range 3 {
			timerCall := timerTrap.MustWait(ctx)
			d := timerCall.Duration
			assert.GreaterOrEqual(t, d, minDur, "duration %v below minimum %v", d, minDur)
			assert.LessOrEqual(t, d, maxDur, "duration %v above maximum %v", d, maxDur)
			timerCall.MustRelease(ctx)
			clock.Advance(d)
		}
	})

	t.Run("FnErrorSkipsInsert", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		// InsertUsageEvent should NOT be called when Fn returns an error.
		// We use .Times(0) on the first tick, then allow it on the second.
		callCount := 0
		db.EXPECT().InsertUsageEvent(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ database.InsertUsageEventParams) error {
				callCount++
				return nil
			}).AnyTimes()

		failFirst := true
		inserter := usage.NewDBInserter(usage.InserterWithClock(clock))
		cron := usage.NewCron(clock, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), db, inserter)
		require.NoError(t, cron.Register(usage.CronJob{
			Name:     "fail-job",
			Interval: 1 * time.Minute,
			Fn: func(_ context.Context) (string, usagetypes.HeartbeatEvent, error) {
				if failFirst {
					failFirst = false
					return "", nil, xerrors.New("synthetic error")
				}
				return "ok-hb", usagetypes.HBAISeats{Count: 1}, nil
			},
		}))

		timerTrap := clock.Trap().NewTimer("fail-job")

		cron.Start(ctx)
		defer cron.Close()
		defer timerTrap.Close()

		// First tick — Fn fails, no insert.
		call1 := timerTrap.MustWait(ctx)
		call1.MustRelease(ctx)
		clock.Advance(call1.Duration)

		// Second tick — Fn succeeds.
		call2 := timerTrap.MustWait(ctx)
		call2.MustRelease(ctx)
		assert.Equal(t, 0, callCount, "insert should not have been called after Fn error")
		clock.Advance(call2.Duration)

		// Wait for the second tick's insert to land.
		require.Eventually(t, func() bool {
			return callCount == 1
		}, testutil.WaitShort, 10*time.Millisecond)
	})

	t.Run("CloseDoesNotPanic", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		inserter := usage.NewDBInserter(usage.InserterWithClock(clock))
		cron := usage.NewCron(clock, slogtest.Make(t, nil), db, inserter)
		require.NoError(t, cron.Register(usage.CronJob{
			Name:     "close-job",
			Interval: 1 * time.Hour,
			Fn: func(_ context.Context) (string, usagetypes.HeartbeatEvent, error) {
				return "x", usagetypes.HBAISeats{Count: 1}, nil
			},
		}))

		timerTrap := clock.Trap().NewTimer("close-job")

		cron.Start(ctx)

		// Wait for the goroutine to create its timer.
		call := timerTrap.MustWait(ctx)
		call.MustRelease(ctx)
		timerTrap.Close()

		// Close should not panic or hang.
		cron.Close()
	})
}

func TestAISeatsHeartbeat(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	db.EXPECT().GetActiveAISeatCount(gomock.Any()).Return(int64(42), nil)

	fn := usage.AISeatsHeartbeat(db)
	id, event, err := fn(ctx)
	require.NoError(t, err)

	// Verify the ID is time-bucketed to 4 hours.
	assert.Regexp(t, `^hb_ai_seats_v1:\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`, id)

	// Verify the event type and count.
	hb, ok := event.(usagetypes.HBAISeats)
	require.True(t, ok)
	assert.Equal(t, uint64(42), hb.Count)
}
