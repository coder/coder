// Package aiseats is the AGPL version the package.
// The actual implementation is in `enterprise/aiseats`.
package aiseats

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

type Reason struct {
	EventType   database.AiSeatUsageReason
	Description string
}

// ReasonAIBridge constructs a reason for usage originating from AI Bridge.
func ReasonAIBridge(description string) Reason {
	return Reason{EventType: database.AiSeatUsageReasonAibridge, Description: description}
}

// ReasonTask constructs a reason for usage originating from tasks.
func ReasonTask(description string) Reason {
	return Reason{EventType: database.AiSeatUsageReasonTask, Description: description}
}

// SeatTracker records AI seat consumption state.
type SeatTracker interface {
	// RecordUsage does not return an error to prevent blocking the user from using
	// AI features. This method is used to record usage, not enforce it.
	RecordUsage(ctx context.Context, userID uuid.UUID, reason Reason)
}

// Noop is an AGPL seat tracker that does nothing.
type Noop struct{}

func (Noop) RecordUsage(context.Context, uuid.UUID, Reason) {}
