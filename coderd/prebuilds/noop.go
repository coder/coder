package prebuilds

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

type NoopReconciler struct{}

func (NoopReconciler) Run(context.Context)                {}
func (NoopReconciler) Stop(context.Context, error)        {}
func (NoopReconciler) ReconcileAll(context.Context) error { return nil }
func (NoopReconciler) SnapshotState(context.Context, database.Store) (*GlobalSnapshot, error) {
	return &GlobalSnapshot{}, nil
}
func (NoopReconciler) ReconcilePreset(context.Context, PresetSnapshot) error { return nil }
func (NoopReconciler) CalculateActions(context.Context, PresetSnapshot) (*ReconciliationActions, error) {
	return &ReconciliationActions{}, nil
}

var DefaultReconciler ReconciliationOrchestrator = NoopReconciler{}

type NoopClaimer struct{}

func (NoopClaimer) Claim(context.Context, uuid.UUID, string, uuid.UUID) (*uuid.UUID, error) {
	// Not entitled to claim prebuilds in AGPL version.
	return nil, ErrAGPLDoesNotSupportPrebuiltWorkspaces
}

func (NoopClaimer) Initiator() uuid.UUID {
	return uuid.Nil
}

var DefaultClaimer Claimer = NoopClaimer{}
