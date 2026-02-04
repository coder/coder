package agentutil

import (
	"context"
	"runtime/debug"

	"cdr.dev/slog/v3"
)

// Go runs the provided function in a goroutine, recovering from panics and
// logging them before re-panicking.
func Go(ctx context.Context, log slog.Logger, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Critical(ctx, "panic in goroutine",
					slog.F("panic", r),
					slog.F("stack", string(debug.Stack())),
				)
				panic(r)
			}
		}()
		fn()
	}()
}
