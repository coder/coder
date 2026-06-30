package chatdebug

import (
	"context"
	"runtime"
	"sync"

	"github.com/google/uuid"
)

type (
	runContextKey   struct{}
	stepContextKey  struct{}
	reuseStepKey    struct{}
	errorEnsurerKey struct{}
	reuseHolder     struct {
		mu     sync.Mutex
		handle *stepHandle
	}
)

// errorRunEnsurer lazily creates a debug run on the first qualifying
// error, so error-free turns write nothing. sync.OnceValues memoizes
// the create func so multiple failing steps in a turn share one run.
type errorRunEnsurer struct {
	get func() (*RunContext, bool)
}

// WithErrorRunEnsurer stores a lazy run creator in ctx. create is
// invoked at most once on the first ensureErrorRun call, and may
// return ok=false to signal the run could not be created.
func WithErrorRunEnsurer(ctx context.Context, create func() (*RunContext, bool)) context.Context {
	if create == nil {
		panic("chatdebug: nil error run ensurer")
	}
	return context.WithValue(ctx, errorEnsurerKey{}, &errorRunEnsurer{
		get: sync.OnceValues(create),
	})
}

// ensureErrorRun returns the lazily-created run context from ctx,
// creating it on first use. It returns ok=false when no ensurer is
// present or the create func declined to produce a run.
func ensureErrorRun(ctx context.Context) (*RunContext, bool) {
	ensurer, ok := ctx.Value(errorEnsurerKey{}).(*errorRunEnsurer)
	if !ok {
		return nil, false
	}
	return ensurer.get()
}

// hasErrorRunEnsurer reports whether ctx carries a lazy run ensurer.
func hasErrorRunEnsurer(ctx context.Context) bool {
	_, ok := ctx.Value(errorEnsurerKey{}).(*errorRunEnsurer)
	return ok
}

// ContextWithRun stores rc in ctx.
//
// Step counter cleanup is reference-counted per RunID: each live
// RunContext increments a counter and runtime.AddCleanup decrements
// it when the struct is garbage collected. Shared state (step
// counters) is only deleted when the last RunContext for a given
// RunID becomes unreachable, preventing premature cleanup when
// multiple RunContext instances share the same RunID.
func ContextWithRun(ctx context.Context, rc *RunContext) context.Context {
	if rc == nil {
		panic("chatdebug: nil RunContext")
	}

	enriched := context.WithValue(ctx, runContextKey{}, rc)
	if rc.RunID != uuid.Nil {
		trackRunRef(rc.RunID)
		runtime.AddCleanup(rc, func(id uuid.UUID) {
			releaseRunRef(id)
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
