package recorder

// Decorator wraps a [Recorder] in additional behavior, returning the wrapping
// [Recorder]. Each decorating recorder in this package has a corresponding
// Decorator constructor, such as [WithLogging], which closes over that
// recorder's own dependencies so that decorators can be composed by [Chain].
type Decorator func(next Recorder) Recorder

// Chain composes decorators into a single [Decorator], to be applied to the
// concrete [Recorder] that terminates the chain:
//
//	rec := Chain(
//		WithLogging(logger),
//		WithTracing(tracer),
//	)(NewWrappedRecorder(clientFn))
//
// Decorators are listed outermost first, in the order records pass through
// them, so the last decorator wraps the terminating recorder directly. A Chain
// is itself a Decorator and may be nested in another Chain. Chaining no
// decorators returns the terminating recorder unchanged.
//
// Chain does not enforce ordering constraints between decorators; see the
// individual Decorator constructors for any position a decorator requires.
func Chain(decorators ...Decorator) Decorator {
	return func(rec Recorder) Recorder {
		for i := len(decorators) - 1; i >= 0; i-- {
			rec = decorators[i](rec)
		}
		return rec
	}
}
