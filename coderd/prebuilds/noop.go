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
func (NoopReconciler) SnapshotState(context.Context, database.Store) (*GlobalSnapshot, error) {
	return &GlobalSnapshot{}, nil
}

func (NoopReconciler) CalculateActions(context.Context, PresetSnapshot) (*ReconciliationActions, error) {
	return &ReconciliationActions{}, nil
}

func (NoopReconciler) ReconcilePreset(context.Context, PresetSnapshot) error {
	return nil
}

var _ ReconciliationOrchestrator = NoopReconciler{}
