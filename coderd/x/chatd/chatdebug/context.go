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
//
// Step counter cleanup is deferred to GC via runtime.AddCleanup on the
// RunContext struct. This avoids the race where context.AfterFunc fires
// on one derived context while another derived context sharing the same
// RunID is still active, which would silently reset step numbering to 1.
func ContextWithRun(ctx context.Context, rc *RunContext) context.Context {
	if rc == nil {
		panic("chatdebug: nil RunContext")
	}

	enriched := context.WithValue(ctx, runContextKey{}, rc)
	if rc.RunID != uuid.Nil {
		// Best-effort cleanup once the RunContext struct becomes
		// unreachable. This is safe with multiple derived contexts
		// because it fires only when *all* references to rc are
		// gone, not when any single context is canceled.
		runtime.AddCleanup(rc, func(id uuid.UUID) {
			CleanupStepCounter(id)
		}, rc.RunID)
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
