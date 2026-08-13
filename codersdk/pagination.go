package codersdk

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

// WorkspacesPageLimit is the largest page size the workspaces list endpoint
// accepts and the page size clients use when reading that endpoint to
// exhaustion. A request that omits limit receives this many rows; a limit of 0
// or greater than this is rejected.
const WorkspacesPageLimit = 1000

// Pagination sets pagination options for the endpoints that support it.
type Pagination struct {
	// AfterID returns all or up to Limit results after the given
	// UUID. This option can be used with or as an alternative to
	// Offset for better performance. To use it as an alternative,
	// set AfterID to the last UUID returned by the previous
	// request.
	AfterID uuid.UUID `json:"after_id,omitempty" format:"uuid"`
	// Limit sets the maximum number of records to be returned in a single
	// page. An endpoint that bounds its page size rejects a limit outside its
	// range; otherwise a limit <= 0 means no limit and every record is
	// returned.
	Limit int `json:"limit,omitempty"`
	// Offset is used to indicate which page to return. An offset of 0
	// returns the first 'limit' number of users.
	// To get the next page, use offset=<limit>*<page_number>.
	// Offset is 0 indexed, so the first record sits at offset 0.
	Offset int `json:"offset,omitempty"`
}

// asRequestOption returns a function that can be used in (*Client).Request.
// It modifies the request query parameters.
func (p Pagination) asRequestOption() RequestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		if p.AfterID != uuid.Nil {
			q.Set("after_id", p.AfterID.String())
		}
		if p.Limit > 0 {
			q.Set("limit", strconv.Itoa(p.Limit))
		}
		if p.Offset > 0 {
			q.Set("offset", strconv.Itoa(p.Offset))
		}
		r.URL.RawQuery = q.Encode()
	}
}
