package prebuilds

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
)

type NoopReconciler struct{}

func NewNoopReconciler() *NoopReconciler {
	return &NoopReconciler{}
}

func (NoopReconciler) RunLoop(context.Context)     {}
func (NoopReconciler) Stop(context.Context, error) {}
func (NoopReconciler) SnapshotState(context.Context, database.Store) (*ReconciliationState, error) {
	return &ReconciliationState{}, nil
}
func (NoopReconciler) DetermineActions(context.Context, PresetState) (*ReconciliationActions, error) {
	return &ReconciliationActions{}, nil
}
func (NoopReconciler) Reconcile(context.Context, PresetState, ReconciliationActions) error {
	return nil
}

var _ ReconciliationOrchestrator = NoopReconciler{}
