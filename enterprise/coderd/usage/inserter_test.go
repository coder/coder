package usage_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
	"github.com/coder/coder/v2/enterprise/coderd/usage"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestInserter(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)
		inserter := usage.NewDBInserter(usage.InserterWithClock(clock))

		now := dbtime.Now()
		events := []struct {
			time  time.Time
			event usagetypes.DiscreteEvent
		}{
			{
				time: now,
				event: usagetypes.DCManagedAgentsV1{
					Count: 1,
				},
			},
			{
				time: now.Add(1 * time.Minute),
				event: usagetypes.DCManagedAgentsV1{
					Count: 2,
				},
			},
		}

		for _, e := range events {
			eventJSON := jsoninate(t, e.event)
			db.EXPECT().InsertUsageEvent(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx interface{}, params database.InsertUsageEventParams) error {
					_, err := uuid.Parse(params.ID)
					assert.NoError(t, err)
					assert.Equal(t, e.event.EventType(), usagetypes.UsageEventType(params.EventType))
					assert.JSONEq(t, eventJSON, string(params.EventData))
					assert.Equal(t, e.time, params.CreatedAt)
					return nil
				},
			).Times(1)

			clock.Set(e.time)
			err := inserter.InsertDiscreteUsageEvent(ctx, db, e.event)
			require.NoError(t, err)
		}
	})

	t.Run("InvalidEvent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		// We should get an error if the event is invalid.
		inserter := usage.NewDBInserter()
		err := inserter.InsertDiscreteUsageEvent(ctx, db, usagetypes.DCManagedAgentsV1{
			Count: 0, // invalid
		})
		assert.ErrorContains(t, err, `invalid "dc_managed_agents_v1" event: count must be greater than 0`)
	})
}
