package codersdk

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type Replica struct {
	// The unique identifier for the replica.
	ID uuid.UUID `json:"id" format:"uuid"`
	// The hostname of the replica.
	Hostname string `json:"hostname"`
	// The timestamp when the replica was first seen.
	CreatedAt time.Time `json:"created_at" format:"date-time"`
	// The accessible address to relay DERP connections.
	RelayAddress string `json:"relay_address"`
	// The region of the replica.
	RegionID int32 `json:"region_id"`
	// The replica error.
	Error string `json:"error"`
	// The latency in microseconds to the database.
	DatabaseLatency int32 `json:"database_latency"`
}

// Replicas fetches the list of replicas.
func (c *Client) Replicas(ctx context.Context) ([]Replica, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/replicas", nil)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var replicas []Replica
	return replicas, ReadBodyAsJSON(res, &replicas)
}
