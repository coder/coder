package chatdebug

import (
	"context"
	"runtime"
	"sync"

	"github.com/google/uuid"
)

type (
	runContextKey  struct{}
	stepContextKey struct{}
	reuseStepKey   struct{}
	reuseHolder    struct {
		mu     sync.Mutex
		handle *stepHandle
	}
)

// ContextWithRun stores rc in ctx.
func ContextWithRun(ctx context.Context, rc *RunContext) context.Context {
	if rc == nil {
		panic("chatdebug: nil RunContext")
	}

	enriched := context.WithValue(ctx, runContextKey{}, rc)
	if rc.RunID != uuid.Nil {
		// Prefer prompt cleanup when the run context is canceled.
		runID := rc.RunID
		context.AfterFunc(enriched, func() {
			CleanupStepCounter(runID)
		})
		// Non-cancelable contexts (for example context.Background in tests)
		// still need a best-effort cleanup path once the run context becomes
		// unreachable.
		runtime.AddCleanup(rc, func(id uuid.UUID) {
			CleanupStepCounter(id)
		}, runID)
	}
	return enriched
}

// RunFromContext returns the debug run context stored in ctx.
func RunFromContext(ctx context.Context) (*RunContext, bool) {
	rc, ok := ctx.Value(runContextKey{}).(*RunContext)
	if !ok {
		return nil, false
	}
	return rc, true
}

// ContextWithStep stores sc in ctx.
func ContextWithStep(ctx context.Context, sc *StepContext) context.Context {
	if sc == nil {
		panic("chatdebug: nil StepContext")
	}
	return context.WithValue(ctx, stepContextKey{}, sc)
}

// StepFromContext returns the debug step context stored in ctx.
func StepFromContext(ctx context.Context) (*StepContext, bool) {
	sc, ok := ctx.Value(stepContextKey{}).(*StepContext)
	if !ok {
		return nil, false
	}
	return sc, true
}

// ReuseStep marks ctx so wrapped model calls under it share one debug step.
func ReuseStep(ctx context.Context) context.Context {
	if holder, ok := reuseHolderFromContext(ctx); ok {
		return context.WithValue(ctx, reuseStepKey{}, holder)
	}
	return context.WithValue(ctx, reuseStepKey{}, &reuseHolder{})
}

func reuseHolderFromContext(ctx context.Context) (*reuseHolder, bool) {
	holder, ok := ctx.Value(reuseStepKey{}).(*reuseHolder)
	if !ok {
		return nil, false
	}
	return holder, true
}
