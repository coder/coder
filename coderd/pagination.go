package coderd

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

func parsePagination(r *http.Request) (p codersdk.Pagination, err error) {
	var (
		afterID = uuid.Nil
		limit   = -1 // Default to no limit and return all results.
		offset  = 0
	)

	if s := r.URL.Query().Get("after_id"); s != "" {
		afterID, err = uuid.Parse(r.URL.Query().Get("after_id"))
		if err != nil {
			return p, xerrors.Errorf("after_id must be a valid uuid: %w", err.Error())
		}
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		limit, err = strconv.Atoi(s)
		if err != nil {
			return p, xerrors.Errorf("limit must be an integer: %w", err.Error())
		}
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		offset, err = strconv.Atoi(s)
		if err != nil {
			return p, xerrors.Errorf("offset must be an integer: %w", err.Error())
		}
	}

	return codersdk.Pagination{
		AfterID: afterID,
		Limit:   limit,
		Offset:  offset,
	}, nil
}
