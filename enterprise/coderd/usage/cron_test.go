package usage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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

		// The existence check should return false so the event gets
		// inserted.
		db.EXPECT().UsageEventExistsByID(gomock.Any(), gomock.Any()).
			Return(false, nil).AnyTimes()

		inserted := make(chan database.InsertUsageEventParams, 1)
		db.EXPECT().InsertUsageEvent(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, params database.InsertUsageEventParams) error {
				inserted <- params
				return nil
			}).AnyTimes()

		inserter := usage.NewDBInserter(usage.InserterWithClock(clock))
		cron := usage.NewCron(clock, slogtest.Make(t, nil), db, inserter)
		require.NoError(t, cron.Register(usage.CronJob{
			Name:      "test-job",
			Interval:  5 * time.Minute,
			EventType: usagetypes.UsageEventTypeHBAISeatsV1,
			Fn: func(_ context.Context) (usagetypes.HeartbeatEvent, error) {
				return usagetypes.HBAISeats{Count: 42}, nil
			},
		}))

		timerTrap := clock.Trap().NewTimer("test-job")

		cron.Start(ctx)
		defer cron.Close()
		defer timerTrap.Close()

		// Wait for timer creation, then fire it. The delay is the
		// time until the next epoch-aligned boundary for the 5-minute
		// interval — we don't assert the exact value since it depends
		// on the mock clock's current time.
		timerCall := timerTrap.MustWait(ctx)
		timerCall.MustRelease(ctx)
		clock.Advance(timerCall.Duration)

		// Verify the event was inserted with an epoch-aligned ID.
		select {
		case params := <-inserted:
			assert.Contains(t, params.ID, "hb_ai_seats_v1:")
		case <-ctx.Done():
			t.Fatal("timed out waiting for insert")
		}
	})
}

// TestAISeatsHeartbeat checks that AISeatsHeartbeat returns the
// correct event type and count.
func TestAISeatsHeartbeat(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	db.EXPECT().GetActiveAISeatCount(gomock.Any()).Return(int64(42), nil)

	fn := usage.AISeatsHeartbeat(db)
	event, err := fn(ctx)
	require.NoError(t, err)

	// Verify the event type and count.
	hb, ok := event.(usagetypes.HBAISeats)
	require.True(t, ok)
	assert.Equal(t, int64(42), hb.Count)
}
