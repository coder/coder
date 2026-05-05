//go:build slim

package chatdebug

import "context"

type (
	runContextKey  struct{}
	stepContextKey struct{}
	reuseStepKey   struct{}
	// reuseHolder is exported in name only: slim chatdebug never instantiates a
	// debug recorder, so the holder carries no step state. The empty struct
	// keeps reflection / type identity consistent with the !slim build.
	reuseHolder struct{}
)

// ContextWithRun stores rc in ctx. The slim build skips the runtime.AddCleanup
// reference counting that the !slim build uses to coordinate step counter
// cleanup, because slim agents never start debug steps.
func ContextWithRun(ctx context.Context, rc *RunContext) context.Context {
	if rc == nil {
		panic("chatdebug: nil RunContext")
	}
	return context.WithValue(ctx, runContextKey{}, rc)
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

// ReuseStep is a no-op in slim builds where there is no model recorder to
// share a step across calls.
func ReuseStep(ctx context.Context) context.Context {
	return ctx
}
