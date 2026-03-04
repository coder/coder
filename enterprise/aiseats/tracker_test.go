package aiseats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	agplaiseats "github.com/coder/coder/v2/coderd/aiseats"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type fakeStore struct {
	calls   int
	lastArg database.UpsertAISeatStateParams
	err     error
}

func (f *fakeStore) UpsertAISeatState(_ context.Context, arg database.UpsertAISeatStateParams) error {
	f.calls++
	f.lastArg = arg
	return f.err
}

func TestRecordUsage(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	now := clock.Now()
	userID := uuid.New()
	store := &fakeStore{}
	tracker := New(store, testutil.Logger(t), clock)

	tracker.RecordUsage(context.Background(), userID, agplaiseats.ReasonAIBridge("used chatgpt"), now)
	require.Equal(t, 1, store.calls)
	require.Equal(t, userID, store.lastArg.UserID)
	require.Equal(t, now, store.lastArg.FirstUsedAt)
	require.Equal(t, database.AiSeatUsageReasonAibridge, store.lastArg.LastEventType)
	require.Equal(t, "used chatgpt", store.lastArg.LastEventDescription)
}

func TestRecordUsageThrottle(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	store := &fakeStore{}
	tracker := New(store, testutil.Logger(t), clock)
	ctx := context.Background()
	userID := uuid.New()

	tracker.RecordUsage(ctx, userID, agplaiseats.ReasonTask("from task"), clock.Now())
	tracker.RecordUsage(ctx, userID, agplaiseats.ReasonTask("from task"), clock.Now())
	require.Equal(t, 1, store.calls)

	_ = clock.Advance(throttleInterval + time.Second)
	tracker.RecordUsage(ctx, userID, agplaiseats.ReasonTask("from task"), clock.Now())
	require.Equal(t, 2, store.calls)
}

func TestRecordUsageErrors(t *testing.T) {
	t.Parallel()

	t.Run("db error", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		store := &fakeStore{err: errors.New("boom")}
		tracker := New(store, testutil.Logger(t), clock)
		userID := uuid.New()
		now := clock.Now()

		tracker.RecordUsage(context.Background(), userID, agplaiseats.ReasonTask("from task"), now)
		tracker.RecordUsage(context.Background(), userID, agplaiseats.ReasonTask("from task"), now)
		require.Equal(t, 1, store.calls)

		_ = clock.Advance(failedRetryInterval + time.Second)
		tracker.RecordUsage(context.Background(), userID, agplaiseats.ReasonTask("from task"), clock.Now())
		require.Equal(t, 2, store.calls)
	})

	t.Run("invalid reason", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		store := &fakeStore{}
		tracker := New(store, testutil.Logger(t), clock)
		var invalid agplaiseats.Reason
		tracker.RecordUsage(context.Background(), uuid.New(), invalid, clock.Now())
		require.Equal(t, 0, store.calls)
	})
}
