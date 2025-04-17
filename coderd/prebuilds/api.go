package prebuilds

import (
	"context"
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

type Reconciler interface {
	// ReconcileAll orchestrates the reconciliation of all prebuilds across all templates.
	// It takes a global snapshot of the system state and then reconciles each preset
	// in parallel, creating or deleting prebuilds as needed to reach their desired states.
	ReconcileAll(ctx context.Context) error
}
