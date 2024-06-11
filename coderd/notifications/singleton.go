package notifications

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/notifications/types"
)

var (
	singleton   Enqueuer
	singletonMu sync.Mutex

	SingletonNotRegisteredErr = xerrors.New("singleton not registered")
)

// RegisterInstance receives a Manager reference to act as a Singleton.
// We use a Singleton to centralize the logic around enqueueing notifications, instead of requiring that an instance
// of the Manager be passed around the codebase.
func RegisterInstance(m Enqueuer) {
	singletonMu.Lock()
	defer singletonMu.Unlock()

	singleton = m
}

// Enqueue queues a notification message for later delivery.
// This is a delegator for the underlying notifications singleton.
func Enqueue(ctx context.Context, userID, templateID uuid.UUID, labels types.Labels, createdBy string, targets ...uuid.UUID) (*uuid.UUID, error) {
	if singleton == nil {
		return nil, SingletonNotRegisteredErr
	}

	return singleton.Enqueue(ctx, userID, templateID, labels, createdBy, targets...)
}
