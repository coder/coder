package prebuilds

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
)

type ReconciliationOrchestrator interface {
	Reconciler

	RunLoop(ctx context.Context)
	Stop(ctx context.Context, cause error)
}

type Reconciler interface {
	// SnapshotState MUST be called inside a repeatable-read tx.
	SnapshotState(ctx context.Context, store database.Store) (*GlobalSnapshot, error)
	// ReconcilePreset MUST be called inside a repeatable-read tx.
	ReconcilePreset(ctx context.Context, snapshot PresetSnapshot) error
	// CalculateActions MUST be called inside a repeatable-read tx.
	CalculateActions(ctx context.Context, state PresetSnapshot) (*ReconciliationActions, error)
}
