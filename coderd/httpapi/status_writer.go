package httpapi

import (
	"bufio"
	"net"
	"net/http"
)

var _ http.ResponseWriter = (*StatusWriter)(nil)
var _ http.Hijacker = (*StatusWriter)(nil)

// StatusWriter intercepts the status of the request and the response body up
// to maxBodySize if Status >= 400. It is guaranteed to be the ResponseWriter
// directly downstream from Middleware.
type StatusWriter struct {
	http.ResponseWriter
	Status       int
	Hijacked     bool
	ResponseBody []byte
}

func (w *StatusWriter) WriteHeader(status int) {
	w.Status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *StatusWriter) Write(b []byte) (int, error) {
	const maxBodySize = 4096

	if w.Status == 0 {
		w.Status = http.StatusOK
	}

	if w.Status >= http.StatusBadRequest {
		// Instantiate the recorded response body to be at most
		// maxBodySize length.
		w.ResponseBody = make([]byte, minInt(len(b), maxBodySize))
		copy(w.ResponseBody, b)
	}

	return w.ResponseWriter.Write(b)
}

// minInt returns the smaller of a or b. This is helpful because math.Min only
// works with float64s.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (w *StatusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.Hijacked = true
	return w.ResponseWriter.(http.Hijacker).Hijack()
}
