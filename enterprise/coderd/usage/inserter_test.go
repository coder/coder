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
					assert.Equal(t, e.time, params.InsertedAt)
					return nil
				},
			).Times(1)

			clock.Set(e.time)
			err := inserter.InsertDiscreteUsageEvent(ctx, db, e.event)
			require.NoError(t, err)
		}
	})

	t.Run("Heartbeat", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)
		inserter := usage.NewDBInserter(usage.InserterWithClock(clock))

		// Heartbeat inserts must store the provided id and createdAt
		// verbatim, while inserted_at is the current time so backfilled
		// buckets are not misdetected as stuck by publish failure detection.
		event := usagetypes.HBAgentRuntime{RuntimeMs: 1234}
		eventJSON := jsoninate(t, event)
		id := "hb_agent_runtime_v1:2025-01-02_03:00:00"
		createdAt := time.Date(2025, 1, 2, 3, 0, 0, 0, time.UTC)
		insertTime := time.Date(2025, 1, 9, 12, 34, 56, 0, time.UTC)
		clock.Set(insertTime)

		db.EXPECT().InsertUsageEvent(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx interface{}, params database.InsertUsageEventParams) error {
				assert.Equal(t, id, params.ID)
				assert.Equal(t, event.EventType(), usagetypes.UsageEventType(params.EventType))
				assert.JSONEq(t, eventJSON, string(params.EventData))
				assert.Equal(t, dbtime.Time(createdAt), params.CreatedAt)
				assert.Equal(t, dbtime.Time(insertTime), params.InsertedAt)
				return nil
			},
		).Times(1)

		err := inserter.InsertHeartbeatUsageEvent(ctx, db, id, createdAt, event)
		require.NoError(t, err)
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

		err = inserter.InsertHeartbeatUsageEvent(ctx, db, "some-id", time.Now(), usagetypes.HBAgentRuntime{
			RuntimeMs: -1, // invalid
		})
		assert.ErrorContains(t, err, `invalid "hb_agent_runtime_v1" event: runtime_ms cannot be negative`)
	})

	t.Run("ZeroCreatedAt", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		ctrl := gomock.NewController(t)
		// The mock store fails the test on any unexpected call, so no insert
		// may reach the database.
		db := dbmock.NewMockStore(ctrl)

		inserter := usage.NewDBInserter()
		err := inserter.InsertHeartbeatUsageEvent(ctx, db, "some-id", time.Time{}, usagetypes.HBAgentRuntime{
			RuntimeMs: 1,
		})
		assert.ErrorContains(t, err, `createdAt must be set for "hb_agent_runtime_v1" event`)
	})
}
