package codersdk

import (
	"time"

	"github.com/google/uuid"
)

// ImmortalStream represents an immortal stream connection
type ImmortalStream struct {
	ID                  uuid.UUID  `json:"id" format:"uuid"`
	Name                string     `json:"name"`
	TCPPort             int        `json:"tcp_port"`
	CreatedAt           time.Time  `json:"created_at" format:"date-time"`
	LastConnectionAt    time.Time  `json:"last_connection_at" format:"date-time"`
	LastDisconnectionAt *time.Time `json:"last_disconnection_at,omitempty" format:"date-time"`
}

// CreateImmortalStreamRequest is the request to create an immortal stream
type CreateImmortalStreamRequest struct {
	TCPPort int `json:"tcp_port"`
}

// ImmortalStreamHeaders are the headers used for immortal stream connections
const (
	HeaderImmortalStreamSequenceNum = "X-Coder-Immortal-Stream-Sequence-Num"
	HeaderUpgrade                   = "Upgrade"
	HeaderConnection                = "Connection"
	UpgradeImmortalStream           = "coder-immortal-stream"
)
