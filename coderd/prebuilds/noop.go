package prebuilds

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
)

type NoopReconciler struct{}

func NewNoopReconciler() *NoopReconciler {
	return &NoopReconciler{}
}

func (NoopReconciler) RunLoop(ctx context.Context)           {}
func (NoopReconciler) Stop(ctx context.Context, cause error) {}
func (NoopReconciler) SnapshotState(ctx context.Context, store database.Store) (*ReconciliationState, error) {
	return &ReconciliationState{}, nil
}
func (NoopReconciler) DetermineActions(ctx context.Context, state PresetState) (*ReconciliationActions, error) {
	return &ReconciliationActions{}, nil
}
func (NoopReconciler) Reconcile(ctx context.Context, state PresetState, actions ReconciliationActions) error {
	return nil
}

var _ ReconciliationOrchestrator = NoopReconciler{}
