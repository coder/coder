package codersdk

import (
	"time"

	"github.com/google/uuid"
)

type Replica struct {
	// ID is the unique identifier for the replica.
	ID uuid.UUID `json:"id"`
	// Hostname is the hostname of the replica.
	Hostname string `json:"hostname"`
	// CreatedAt is when the replica was first seen.
	CreatedAt time.Time `json:"created_at"`
	// Active determines whether the replica is online.
	Active bool `json:"active"`
	// RelayAddress is the accessible address to relay DERP connections.
	RelayAddress string `json:"relay_address"`
	// Error is the error.
	Error string `json:"error"`
}
