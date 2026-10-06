package replaybody

import (
	"bufio"
	"net"
	"net/http"
)

// responseWriter removes the Connection: close default that NewRequest sets
// when the final header is written after source EOF. Before EOF the header
// stays, so the server replies without draining the unfinished upload and
// closes the connection instead of reusing it. Only WriteHeader, Write, and
// Flush commit the header through this writer; an implicit reply written by
// the server after the handler returns keeps the default.
type responseWriter struct {
	http.ResponseWriter
	source      *bodySource
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(code int) {
	// 1xx replies are informational, and 101 carries Connection: Upgrade.
	if !w.wroteHeader && code >= http.StatusOK {
		w.wroteHeader = true
		if w.source.eof.Load() {
			w.Header().Del("Connection")
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

// Flush commits the implicit header before flushing. Handlers reach it through
// http.Flusher or http.ResponseController.
func (w *responseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// Hijack satisfies the repository rule that wrappers implement http.Hijacker.
func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
