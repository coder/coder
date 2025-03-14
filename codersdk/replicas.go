package codersdk
import (
	"fmt"
	"errors"
	"context"
	"encoding/json"
	"net/http"
	"time"
	"github.com/google/uuid"
)
type Replica struct {
	// ID is the unique identifier for the replica.
	ID uuid.UUID `json:"id" format:"uuid"`
	// Hostname is the hostname of the replica.
	Hostname string `json:"hostname"`
	// CreatedAt is the timestamp when the replica was first seen.
	CreatedAt time.Time `json:"created_at" format:"date-time"`
	// RelayAddress is the accessible address to relay DERP connections.
	RelayAddress string `json:"relay_address"`
	// RegionID is the region of the replica.
	RegionID int32 `json:"region_id"`
	// Error is the replica error.
	Error string `json:"error"`
	// DatabaseLatency is the latency in microseconds to the database.
	DatabaseLatency int32 `json:"database_latency"`
}
// Replicas fetches the list of replicas.
func (c *Client) Replicas(ctx context.Context) ([]Replica, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/replicas", nil)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var replicas []Replica
	return replicas, json.NewDecoder(res.Body).Decode(&replicas)
}
