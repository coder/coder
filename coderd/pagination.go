package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// ParsePagination extracts pagination query params from the http request.
// If an error is encountered, the error is written to w and ok is set to false.
//
// This delegates to httpapi.ParsePagination, which packages that cannot import
// coderd can use directly.
func ParsePagination(w http.ResponseWriter, r *http.Request) (p codersdk.Pagination, ok bool) {
	return httpapi.ParsePagination(w, r)
}
