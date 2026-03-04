package aiseats

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	agplaiseats "github.com/coder/coder/v2/coderd/aiseats"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
)

type store interface {
	UpsertAISeatState(ctx context.Context, arg database.UpsertAISeatStateParams) error
}

// throttleInterval is the minimum time between DB writes for the same user. This
// is to prevent ai seat tracking from consuming more db resources. It is 30
// minutes more than the db interval to ensure each insert has a better chance to
// take effect in the db.
//
// These events are not critical to be recorded in real time, so we can afford to
// skip almost all of them.
const (
	throttleInterval    = (6 * time.Hour) + time.Minute*30
	failedRetryInterval = 10 * time.Minute
)

// SeatTracker records current AI seat state for users.
type SeatTracker struct {
	db     store
	logger slog.Logger
	clock  quartz.Clock

	mu         sync.Mutex
	retryAfter map[uuid.UUID]time.Time
}

func New(db store, logger slog.Logger, clock quartz.Clock) *SeatTracker {
	if clock == nil {
		clock = quartz.NewReal()
	}
	return &SeatTracker{db: db, logger: logger, clock: clock, retryAfter: make(map[uuid.UUID]time.Time)}
}

// skipRecord returns true when the user is still in the retry cooldown
// window and we should skip a DB write attempt.
func (t *SeatTracker) skipRecord(userID uuid.UUID, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	retryAfter, ok := t.retryAfter[userID]
	return ok && now.Before(retryAfter)
}

// recordThrottle sets the next time when DB writes for this user are allowed.
func (t *SeatTracker) recordThrottle(userID uuid.UUID, now time.Time, d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.retryAfter[userID] = now.Add(d)
}

func (t *SeatTracker) RecordUsage(ctx context.Context, userID uuid.UUID, reason agplaiseats.Reason, at time.Time) {
	now := t.clock.Now()
	if t.skipRecord(userID, now) {
		return
	}

	eventType, description, ok := agplaiseats.ReasonValues(reason)
	if !ok {
		t.logger.Warn(ctx, "invalid AI seat usage reason", slog.F("user_id", userID), slog.F("reason_type", fmt.Sprintf("%T", reason)))
		return
	}

	err := t.db.UpsertAISeatState(ctx, database.UpsertAISeatStateParams{
		UserID:               userID,
		FirstUsedAt:          at,
		LastEventType:        eventType,
		LastEventDescription: description,
	})
	if err != nil {
		t.logger.Warn(ctx, "upsert AI seat state", slog.Error(err), slog.F("user_id", userID), slog.F("event_type", eventType))
		t.recordThrottle(userID, now, failedRetryInterval)
		return
	}

	t.recordThrottle(userID, now, throttleInterval)
}
