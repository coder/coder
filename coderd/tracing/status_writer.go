package tracing

import (
	"bufio"
	"net"
	"net/http"

	"golang.org/x/xerrors"
)

var (
	_ http.ResponseWriter = (*StatusWriter)(nil)
	_ http.Hijacker       = (*StatusWriter)(nil)
)

// StatusWriter intercepts the status of the request and the response body up
// to maxBodySize if Status >= 400. It is guaranteed to be the ResponseWriter
// directly downstream from Middleware.
type StatusWriter struct {
	http.ResponseWriter
	Status       int
	Hijacked     bool
	responseBody []byte

	wroteHeader bool
}

func StatusWriterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		sw := &StatusWriter{ResponseWriter: rw}
		next.ServeHTTP(sw, r)
	})
}

func (w *StatusWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.Status = status
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *StatusWriter) Write(b []byte) (int, error) {
	const maxBodySize = 4096

	if !w.wroteHeader {
		w.Status = http.StatusOK
		w.wroteHeader = true
	}

	if w.Status >= http.StatusBadRequest {
		// This is technically wrong as multiple calls to write
		// will simply overwrite w.ResponseBody but given that
		// we typically only write to the response body once
		// and this field is only used for logging I'm leaving
		// this as-is.
		w.responseBody = make([]byte, minInt(len(b), maxBodySize))
		copy(w.responseBody, b)
	}

	return w.ResponseWriter.Write(b)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (w *StatusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, xerrors.Errorf("%T is not a http.Hijacker", w.ResponseWriter)
	}
	w.Hijacked = true

	return hijacker.Hijack()
}

func (w *StatusWriter) ResponseBody() []byte {
	return w.responseBody
}

func (w *StatusWriter) Flush() {
	f, ok := w.ResponseWriter.(http.Flusher)
	if !ok {
		panic("http.ResponseWriter is not http.Flusher")
	}
	f.Flush()
}
