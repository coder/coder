package coderd

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// parsePagination extracts pagination query params from the http request.
// If an error is encountered, the error is written to w and ok is set to false.
func parsePagination(w http.ResponseWriter, r *http.Request) (p codersdk.Pagination, ok bool) {
	var (
		afterID = uuid.Nil
		limit   = -1 // Default to no limit and return all results.
		offset  = 0
	)

	var err error
	if s := r.URL.Query().Get("after_id"); s != "" {
		afterID, err = uuid.Parse(r.URL.Query().Get("after_id"))
		if err != nil {
			httpapi.Write(w, http.StatusBadRequest, httpapi.Response{
				Message: fmt.Sprintf("after_id must be a valid uuid: %s", err.Error()),
			})
			return p, false
		}
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		limit, err = strconv.Atoi(s)
		if err != nil {
			httpapi.Write(w, http.StatusBadRequest, httpapi.Response{
				Message: fmt.Sprintf("limit must be an integer: %s", err.Error()),
			})
			return p, false
		}
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		offset, err = strconv.Atoi(s)
		if err != nil {
			httpapi.Write(w, http.StatusBadRequest, httpapi.Response{
				Message: fmt.Sprintf("offset must be an integer: %s", err.Error()),
			})
			return p, false
		}
	}

	return codersdk.Pagination{
		AfterID: afterID,
		Limit:   limit,
		Offset:  offset,
	}, true
}
