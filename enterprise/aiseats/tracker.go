package aiseats

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	agplaiseats "github.com/coder/coder/v2/coderd/aiseats"
	"github.com/coder/coder/v2/coderd/database"
)

type store interface {
	UpsertAISeatState(ctx context.Context, arg database.UpsertAISeatStateParams) error
}

// SeatTracker records current AI seat state for users.
type SeatTracker struct {
	db     store
	logger slog.Logger
}

func New(db store, logger slog.Logger) *SeatTracker {
	return &SeatTracker{db: db, logger: logger}
}

func (t *SeatTracker) RecordUsage(ctx context.Context, userID uuid.UUID, reason agplaiseats.Reason, at time.Time) {
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
	}
}
