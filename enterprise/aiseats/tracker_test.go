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
)

type fakeStore struct {
	lastArg database.UpsertAISeatStateParams
	err     error
}

func (f *fakeStore) UpsertAISeatState(_ context.Context, arg database.UpsertAISeatStateParams) error {
	f.lastArg = arg
	return f.err
}

func TestRecordUsage(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	userID := uuid.New()
	store := &fakeStore{}
	tracker := New(store, testutil.Logger(t))

	tracker.RecordUsage(context.Background(), userID, agplaiseats.ReasonAIBridge("used chatgpt"), now)
	require.Equal(t, userID, store.lastArg.UserID)
	require.Equal(t, now, store.lastArg.FirstUsedAt)
	require.Equal(t, database.AiSeatUsageReasonAibridge, store.lastArg.LastEventType)
	require.Equal(t, "used chatgpt", store.lastArg.LastEventDescription)
}

func TestRecordUsageErrors(t *testing.T) {
	t.Parallel()

	t.Run("db error", func(t *testing.T) {
		t.Parallel()

		store := &fakeStore{err: errors.New("boom")}
		tracker := New(store, testutil.Logger(t))
		tracker.RecordUsage(context.Background(), uuid.New(), agplaiseats.ReasonTask("from task"), time.Now().UTC())
	})

	t.Run("invalid reason", func(t *testing.T) {
		t.Parallel()

		tracker := New(&fakeStore{}, testutil.Logger(t))
		var invalid agplaiseats.Reason
		tracker.RecordUsage(context.Background(), uuid.New(), invalid, time.Now().UTC())
	})
}
