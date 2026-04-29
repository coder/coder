package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
