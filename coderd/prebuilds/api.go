package prebuilds

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

// ReconciliationOrchestrator manages the lifecycle of prebuild reconciliation.
// It runs a continuous loop to check and reconcile prebuild states, and can be stopped gracefully.
type ReconciliationOrchestrator interface {
	Reconciler

	// RunLoop starts a continuous reconciliation loop that periodically calls ReconcileAll
	// to ensure all prebuilds are in their desired states. The loop runs until the context
	// is canceled or Stop is called.
	RunLoop(ctx context.Context)

	// Stop gracefully shuts down the orchestrator with the given cause.
	// The cause is used for logging and error reporting.
	Stop(ctx context.Context, cause error)
}

// Reconciler defines the core operations for managing prebuilds.
// It provides both high-level orchestration (ReconcileAll) and lower-level operations
// for more fine-grained control (SnapshotState, ReconcilePreset, CalculateActions).
// All database operations must be performed within repeatable-read transactions
// to ensure consistency.
type Reconciler interface {
	// ReconcileAll orchestrates the reconciliation of all prebuilds across all templates.
	// It takes a global snapshot of the system state and then reconciles each preset
	// in parallel, creating or deleting prebuilds as needed to reach their desired states.
	// For more fine-grained control, you can use the lower-level methods SnapshotState
	// and ReconcilePreset directly.
	ReconcileAll(ctx context.Context) error

	// SnapshotState captures the current state of all prebuilds across templates.
	// It creates a global database snapshot that can be viewed as a collection of PresetSnapshots,
	// each representing the state of prebuilds for a specific preset.
	// MUST be called inside a repeatable-read transaction.
	SnapshotState(ctx context.Context, store database.Store) (*GlobalSnapshot, error)

	// ReconcilePreset handles a single PresetSnapshot, determining and executing
	// the required actions (creating or deleting prebuilds) based on the current state.
	// MUST be called inside a repeatable-read transaction.
	ReconcilePreset(ctx context.Context, snapshot PresetSnapshot) error

	// CalculateActions determines what actions are needed to reconcile a preset's prebuilds
	// to their desired state. This includes creating new prebuilds, deleting excess ones,
	// or waiting due to backoff periods.
	// MUST be called inside a repeatable-read transaction.
	CalculateActions(ctx context.Context, state PresetSnapshot) (*ReconciliationActions, error)
}

type Claimer interface {
	Claim(ctx context.Context, store database.Store, userID uuid.UUID, name string, presetID uuid.UUID) (*uuid.UUID, error)
	Initiator() uuid.UUID
}
