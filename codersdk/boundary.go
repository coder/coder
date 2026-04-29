package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type BoundarySession struct {
	ID              uuid.UUID `json:"id" format:"uuid"`
	WorkspaceID     uuid.UUID `json:"workspace_id" format:"uuid"`
	OwnerID         uuid.UUID `json:"owner_id" format:"uuid"`
	ConfinedProcess string    `json:"confined_process"`
	StartedAt       time.Time `json:"started_at" format:"date-time"`
}

// BoundarySessionByID returns a boundary session by its ID.
func (c *Client) BoundarySessionByID(ctx context.Context, id uuid.UUID) (BoundarySession, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/boundary/sessions/%s", id), nil)
	if err != nil {
		return BoundarySession{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return BoundarySession{}, ReadBodyAsError(res)
	}
	var session BoundarySession
	return session, json.NewDecoder(res.Body).Decode(&session)
}

// BoundaryLog represents a single audit event from a Boundary proxy.
type BoundaryLog struct {
	ID             uuid.UUID  `json:"id" format:"uuid"`
	SessionID      uuid.UUID  `json:"session_id" format:"uuid"`
	SequenceNumber int64      `json:"sequence_number"`
	Allowed        bool       `json:"allowed"`
	Time           time.Time  `json:"time" format:"date-time"`
	Proto          string     `json:"proto"`
	Method         string     `json:"method"`
	Detail         string     `json:"detail"`
	MatchedRule    *string    `json:"matched_rule"`
	CapturedAt     *time.Time `json:"captured_at,omitempty" format:"date-time"`
}

// BoundarySessionLogsResponse is the response for GET
// /api/v2/boundary/sessions/{id}/logs.
type BoundarySessionLogsResponse struct {
	Results []BoundaryLog `json:"results"`
}

// BoundarySessionLogsParams are query parameters for listing boundary
// session logs.
type BoundarySessionLogsParams struct {
	// SeqAfter is an exclusive lower bound on sequence_number.
	// Only logs with sequence_number > SeqAfter are returned.
	SeqAfter *int64 `json:"seq_after,omitempty"`
	// SeqBefore is an exclusive upper bound on sequence_number.
	// Only logs with sequence_number < SeqBefore are returned.
	SeqBefore *int64 `json:"seq_before,omitempty"`
	// Limit caps the number of returned rows. Defaults to 100.
	Limit *int32 `json:"limit,omitempty"`
}

// BoundarySessionLogs returns boundary audit logs for the given session,
// sorted by sequence number ascending.
func (c *Client) BoundarySessionLogs(ctx context.Context, sessionID uuid.UUID, params BoundarySessionLogsParams) (BoundarySessionLogsResponse, error) {
	qp := make(map[string]string)
	if params.SeqAfter != nil {
		qp["seq_after"] = strconv.FormatInt(*params.SeqAfter, 10)
	}
	if params.SeqBefore != nil {
		qp["seq_before"] = strconv.FormatInt(*params.SeqBefore, 10)
	}
	if params.Limit != nil {
		qp["limit"] = strconv.FormatInt(int64(*params.Limit), 10)
	}

	reqURL := fmt.Sprintf("/api/v2/boundary/sessions/%s/logs", sessionID.String())
	if len(qp) > 0 {
		first := true
		for k, v := range qp {
			if first {
				reqURL += "?"
				first = false
			} else {
				reqURL += "&"
			}
			reqURL += k + "=" + v
		}
	}

	res, err := c.Request(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return BoundarySessionLogsResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return BoundarySessionLogsResponse{}, ReadBodyAsError(res)
	}

	var resp BoundarySessionLogsResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
