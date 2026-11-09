package routing

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/util/xurl"
)

// InvalidPathMessage is the response body for requests rejected by
// [RejectInvalidForwardPath].
const InvalidPathMessage = "invalid request path"

// RejectInvalidForwardPath wraps next and responds with 400 to requests whose
// path is unsafe to forward upstream, as reported by
// [xurl.ContainsEncodedPath], before any routing or upstream request.
func RejectInvalidForwardPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if xurl.ContainsEncodedPath(r.URL) {
			http.Error(w, InvalidPathMessage, http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}
