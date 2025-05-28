//go:build slim

package httpapi

import (
	"context"
	"net/http"
)

func SetAuthzCheckRecorderHeader(ctx context.Context, rw http.ResponseWriter) {
	// There's no RBAC on the agent API, so this is separately defined to
	// avoid importing the RBAC package, which is a large dependency.
}
