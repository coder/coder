package chatdebug

import (
	"context"
	"runtime"
	"sync"

	"github.com/google/uuid"
)

type (
	runContextKey struct{}
	reuseStepKey  struct{}
	reuseHolder   struct {
		mu     sync.Mutex
		handle *stepHandle
	}
)

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
