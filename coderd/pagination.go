package coderd

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// MaxPaginationLimit is the largest page size ParsePagination accepts. An
// omitted limit resolves to this value. A limit that is set must be a positive
// integer no greater than this, so the resulting query is always bounded.
const MaxPaginationLimit = 100

// ParsePagination extracts pagination query params from the http request.
// If an error is encountered, the error is written to w and ok is set to false.
func ParsePagination(w http.ResponseWriter, r *http.Request) (p codersdk.Pagination, ok bool) {
	ctx := r.Context()
	queryParams := r.URL.Query()
	parser := httpapi.NewQueryParamParser()
	params := codersdk.Pagination{
		AfterID: parser.UUID(queryParams, uuid.Nil, "after_id"),
		Offset:  int(parser.PositiveInt32(queryParams, 0, "offset")),
	}
	limitErrsBefore := len(parser.Errors)
	params.Limit = int(parser.PositiveInt32(queryParams, 0, "limit"))
	limitParsed := len(parser.Errors) == limitErrsBefore
	// An omitted limit resolves to MaxPaginationLimit so the downstream query is
	// never unbounded. A limit that is set must be a positive integer no greater
	// than MaxPaginationLimit; otherwise it is rejected rather than clamped.
	switch {
	case queryParams.Get("limit") == "":
		params.Limit = MaxPaginationLimit
	case limitParsed && (params.Limit < 1 || params.Limit > MaxPaginationLimit):
		parser.Errors = append(parser.Errors, codersdk.ValidationError{
			Field:  "limit",
			Detail: fmt.Sprintf("Query param \"limit\" must be a positive integer no greater than %d.", MaxPaginationLimit),
		})
	}
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return params, false
	}

	return params, true
}
