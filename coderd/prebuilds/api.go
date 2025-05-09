package prebuilds

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

var (
	ErrNoClaimablePrebuiltWorkspaces        = xerrors.New("no claimable prebuilt workspaces found")
	ErrAGPLDoesNotSupportPrebuiltWorkspaces = xerrors.New("prebuilt workspaces functionality is not supported under the AGPL license")
)

// ReconciliationOrchestrator manages the lifecycle of prebuild reconciliation.
// It runs a continuous loop to check and reconcile prebuild states, and can be stopped gracefully.
type ReconciliationOrchestrator interface {
	Reconciler

	// Run starts a continuous reconciliation loop that periodically calls ReconcileAll
	// to ensure all prebuilds are in their desired states. The loop runs until the context
	// is canceled or Stop is called.
	Run(ctx context.Context)

	// Stop gracefully shuts down the orchestrator with the given cause.
	// The cause is used for logging and error reporting.
	Stop(ctx context.Context, cause error)
}

type Reconciler interface {
	StateSnapshotter

	// ReconcileAll orchestrates the reconciliation of all prebuilds across all templates.
	// It takes a global snapshot of the system state and then reconciles each preset
	// in parallel, creating or deleting prebuilds as needed to reach their desired states.
	ReconcileAll(ctx context.Context) error
}

// StateSnapshotter defines the operations necessary to capture workspace prebuilds state.
type StateSnapshotter interface {
	// SnapshotState captures the current state of all prebuilds across templates.
	// It creates a global database snapshot that can be viewed as a collection of PresetSnapshots,
	// each representing the state of prebuilds for a specific preset.
	// MUST be called inside a repeatable-read transaction.
	SnapshotState(ctx context.Context, store database.Store) (*GlobalSnapshot, error)
}

type Claimer interface {
	Claim(ctx context.Context, userID uuid.UUID, name string, presetID uuid.UUID) (*uuid.UUID, error)
	Initiator() uuid.UUID
}
