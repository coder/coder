package aiseats

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	agplaiseats "github.com/coder/coder/v2/coderd/aiseats"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
)

type store interface {
	UpsertAISeatState(ctx context.Context, arg database.UpsertAISeatStateParams) (bool, error)
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
	// failedThrottleInterval exists to prevent a transient error from causing no
	// usage to be recorded. Still debounce.
	failedThrottleInterval = 30 * time.Minute
)

// SeatTracker records current AI seat state for users.
type SeatTracker struct {
	db      store
	logger  slog.Logger
	clock   quartz.Clock
	auditor *atomic.Pointer[audit.Auditor]

	mu         sync.RWMutex
	retryAfter map[uuid.UUID]time.Time
}

func New(db store, logger slog.Logger, clock quartz.Clock, auditor *atomic.Pointer[audit.Auditor]) *SeatTracker {
	if clock == nil {
		clock = quartz.NewReal()
	}
	return &SeatTracker{db: db, logger: logger, clock: clock, auditor: auditor, retryAfter: make(map[uuid.UUID]time.Time)}
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

	isNew, err := t.db.UpsertAISeatState(ctx, database.UpsertAISeatStateParams{
		UserID:               userID,
		FirstUsedAt:          now,
		LastEventType:        reason.EventType,
		LastEventDescription: reason.Description,
	})
	if err != nil {
		t.logger.Warn(ctx, "upsert AI seat state", slog.Error(err), slog.F("user_id", userID), slog.F("event_type", reason.EventType))
		t.recordThrottle(userID, now, failedThrottleInterval)
		return
	}

	t.recordThrottle(userID, now, throttleInterval)
	if isNew && t.auditor != nil {
		// Record an audit log for the first time a user uses an AI seat.
		auditor := t.auditor.Load()
		if auditor == nil || *auditor == nil {
			return
		}
		audit.BackgroundAudit[database.AiSeatState](ctx, &audit.BackgroundAuditParams[database.AiSeatState]{
			Audit:  *auditor,
			Log:    t.logger,
			UserID: userID,
			Time:   now,
			Action: database.AuditActionCreate,
			New: database.AiSeatState{
				UserID:               userID,
				FirstUsedAt:          now,
				LastUsedAt:           now,
				LastEventType:        reason.EventType,
				LastEventDescription: reason.Description,
				UpdatedAt:            now,
			},
		})
	}
}
