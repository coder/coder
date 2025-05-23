//go:build !slim

package httpapi

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/rbac"
)

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
		rw.Header().Set("x-authz-checks", rec.String())
	}
}
