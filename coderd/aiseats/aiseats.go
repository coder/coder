package aiseats

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

// Reason describes what AI event consumed the seat.
type Reason interface {
	isReason()
}

type reason struct {
	eventType   database.AiSeatUsageReason
	description string
}

func (reason) isReason() {}

// ReasonValues extracts storage values from a Reason.
func ReasonValues(r Reason) (database.AiSeatUsageReason, string, bool) {
	rr, ok := r.(reason)
	if !ok {
		return "", "", false
	}
	return rr.eventType, rr.description, true
}

// ReasonAIBridge constructs a reason for usage originating from AI Bridge.
func ReasonAIBridge(description string) Reason {
	return reason{eventType: database.AiSeatUsageReasonAibridge, description: description}
}

// ReasonTask constructs a reason for usage originating from tasks.
func ReasonTask(description string) Reason {
	return reason{eventType: database.AiSeatUsageReasonTask, description: description}
}

// SeatTracker records AI seat consumption state.
type SeatTracker interface {
	RecordUsage(ctx context.Context, userID uuid.UUID, reason Reason, at time.Time)
}

// Noop is an AGPL seat tracker that does nothing.
type Noop struct{}

func (Noop) RecordUsage(context.Context, uuid.UUID, Reason, time.Time) {}
