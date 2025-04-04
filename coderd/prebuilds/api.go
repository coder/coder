package prebuilds

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

type ReconciliationOrchestrator interface {
	Reconciler

	RunLoop(ctx context.Context)
	Stop(ctx context.Context, cause error)
}

type Reconciler interface {
	// SnapshotState MUST be called inside a repeatable-read tx.
	SnapshotState(ctx context.Context, store database.Store) (*ReconciliationState, error)
	// DetermineActions MUST be called inside a repeatable-read tx.
	DetermineActions(ctx context.Context, state PresetState) (*ReconciliationActions, error)
	// Reconcile MUST be called inside a repeatable-read tx.
	Reconcile(ctx context.Context, state PresetState, actions ReconciliationActions) error
}

type Claimer interface {
	Claim(ctx context.Context, store database.Store, userID uuid.UUID, name string, presetID uuid.UUID) (*uuid.UUID, error)
	Initiator() uuid.UUID
}
