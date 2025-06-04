package httpapi

import "net/http"

// NoopResponseWriter is a response writer that does nothing.
type NoopResponseWriter struct{}

func (NoopResponseWriter) Header() http.Header         { return http.Header{} }
func (NoopResponseWriter) Write(p []byte) (int, error) { return len(p), nil }
func (NoopResponseWriter) WriteHeader(int)             {}
