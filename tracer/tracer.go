// Package tracer provides application tracing and performance monitoring utilities.
package tracer

import (
	"net/http"

	chitrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/go-chi/chi.v5"
	ddtracer "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// Starts the global tracer.
// Call as early as possible in the application to capture the most data.
// You must call Stop() to ensure all spans are properly flushed before exit.
//
// Common Usage:
//
// tracer.Start()
// defer tracer.Stop()
//
func Start() {
	ddtracer.Start()
}

// Stops the global tracer.
// Should be called at the end of the program to flush the remaining traces.
func Stop() {
	ddtracer.Stop()
}

// Middleware will wrap a trace span around the next http.Handler.
func Middleware() func(next http.Handler) http.Handler {
	return chitrace.Middleware()
}
