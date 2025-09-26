//go:build !slim

package httpapi

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/rbac"
)

// The x-authz-checks header can end up being >10KB in size on certain queries.
// Many HTTP clients will fail if a header or the response head as a whole is
// too long to prevent malicious responses from consuming all of the client's
// memory. I've seen reports that browsers have this limit as low as 4KB for the
// entire response head, so we limit this header to a little less than 2KB,
// ensuring there's still plenty of room for the usual smaller headers.
const maxHeaderLength = 2000

// This is defined separately in slim builds to avoid importing the rbac
// package, which is a large dependency.
func SetAuthzCheckRecorderHeader(ctx context.Context, rw http.ResponseWriter) {
	if rec, ok := rbac.GetAuthzCheckRecorder(ctx); ok {
		// If you're here because you saw this header in a response, and you're
		// trying to investigate the code, here are a couple of notable things
		// for you to know:
		// - If any of the checks are `false`, they might not represent the whole
		//   picture. There could be additional checks that weren't performed,
		//   because processing stopped after the failure.
		// - The checks are recorded by the `authzRecorder` type, which is
		//   configured on server startup for development and testing builds.
		// - If this header is missing from a response, make sure the response is
		//   being written by calling `httpapi.Write`!
		checks := rec.String()
		if len(checks) > maxHeaderLength {
			checks = checks[:maxHeaderLength]
			checks += "<truncated>"
		}
		rw.Header().Set("x-authz-checks", checks)
	}
}
