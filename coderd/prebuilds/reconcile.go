package prebuilds

import (
	"context"
)

type noopReconciler struct{}

func NewNoopReconciler() Reconciler {
	return &noopReconciler{}
}

func (noopReconciler) RunLoop(context.Context)            {}
func (noopReconciler) Stop(context.Context, error)    {}
func (noopReconciler) ReconcileAll(context.Context) error { return nil }
func (noopReconciler) ReconcileTemplate() error           { return nil }

var _ Reconciler = noopReconciler{}
