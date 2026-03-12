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
// is to prevent ai seat tracking from consuming more db resources.
//
// These events are not critical to be recorded in real time, so we can afford to
// skip almost all of them. The first write is the most important, as it
// indicates a seat is consumed. Subsequent writes are purely informative and has
// no functional impact.
const (
	throttleInterval = 6 * time.Hour
	// failedRetryInterval exists to prevent a transient error from causing no
	// usage to be recorded. Still debounce.
	failedRetryInterval = 30 * time.Minute
)

// SeatTracker records current AI seat state for users.
type SeatTracker struct {
	db     store
	logger slog.Logger
	clock  quartz.Clock

	mu         sync.RWMutex
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
	t.mu.RLock()
	defer t.mu.RUnlock()

	retryAfter, ok := t.retryAfter[userID]
	return ok && now.Before(retryAfter)
}

// recordThrottle sets the next time when DB writes for this user are allowed.
func (t *SeatTracker) recordThrottle(userID uuid.UUID, now time.Time, d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.retryAfter[userID] = now.Add(d)
}

// RecordUsage will record the AI seat usage for the user. There is a race condition between
// checking if the user should be recorded or throttled and actually recording. This is fine, as
// it just means we record the usage twice.
// The throttle just exists to prevent excessive database queries.
func (t *SeatTracker) RecordUsage(ctx context.Context, userID uuid.UUID, reason agplaiseats.Reason) {
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
		FirstUsedAt:          now,
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
