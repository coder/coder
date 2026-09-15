package recorder

// Middleware wraps a [Recorder] in additional behavior, returning the wrapping
// [Recorder]. Each decorating recorder in this package has a corresponding
// Middleware constructor, such as [WithLogging], which closes over that
// recorder's own dependencies so that decorators can be composed by [ChainMiddleware].
type Middleware func(next Recorder) Recorder

// ChainMiddleware composes a slice of middleware into a single [Middleware].
//
// The example below wraps a recorder, which does no logging or tracing of its own
// in middleware that adds logging and tracing for every call. In the order provided,
// logging will happen first, then tracing, and then the call to the recorder.
//
//	middleware := ChainMiddleware(
//		WithLogging(logger),
//		WithTracing(tracer),
//	)
//
//	recorder := NewWrappedRecorder(clientFn)
//
//	wrappedRecorder := middleware(recorder)
//
// Middleware are listed outermost first, so the last middleware wraps the recorder.
//
// ChainMiddleware does not enforce ordering constraints between decorators; see the
// individual Decorator constructors for any position a decorator requires.
func ChainMiddleware(middleware ...Middleware) Middleware {
	return func(rec Recorder) Recorder {
		for i := len(middleware) - 1; i >= 0; i-- {
			rec = middleware[i](rec)
		}
		return rec
	}
}
