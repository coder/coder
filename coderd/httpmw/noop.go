package httpmw

import "net/http"

// noopResponseWriter is a response writer that does nothing.
type noopResponseWriter struct{}

func (noopResponseWriter) Header() http.Header         { return http.Header{} }
func (noopResponseWriter) Write(p []byte) (int, error) { return len(p), nil }
func (noopResponseWriter) WriteHeader(int)             {}
