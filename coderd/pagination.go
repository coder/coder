package coderd

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// parsePagination extracts pagination query params from the http request.
// If an error is encountered, the error is written to w and ok is set to false.
func parsePagination(w http.ResponseWriter, r *http.Request) (p codersdk.Pagination, ok bool) {
	parser := httpapi.NewQueryParamParser()
	params := codersdk.Pagination{
		AfterID: parser.UUID(r, uuid.Nil, "after_id"),
		// Limit default to "-1" which returns all results
		Limit:  parser.Int(r, -1, "limit"),
		Offset: parser.Int(r, 0, "offset"),
	}
	if len(parser.Errors) > 0 {
		httpapi.Write(w, http.StatusBadRequest, httpapi.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return params, false
	}

	return params, true
}
