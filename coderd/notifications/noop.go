package notifications

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/notifications/types"
)

type NoopManager struct{}

// NewNoopManager builds a NoopManager which is used to fulfill the contract for enqueuing notifications, if ExperimentNotifications is not set.
func NewNoopManager() *NoopManager {
	return &NoopManager{}
}

func (*NoopManager) Enqueue(context.Context, uuid.UUID, uuid.UUID, types.Labels, string, ...uuid.UUID) (*uuid.UUID, error) {
	// nolint:nilnil // irrelevant.
	return nil, nil
}
