package trafficgen

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type Config struct {
	// AgentID is the workspace agent ID to which to connect.
	AgentID uuid.UUID `json:"agent_id"`
	// BytesPerSecond is the number of bytes to send to the agent.

	BytesPerSecond int64 `json:"bytes_per_second"`

	// Duration is the total duration for which to send traffic to the agent.
	Duration time.Duration `json:"duration"`

	// TicksPerSecond specifies how many times per second we send traffic.
	TicksPerSecond int64 `json:"ticks_per_second"`
}

func (c Config) Validate() error {
	if c.AgentID == uuid.Nil {
		return xerrors.Errorf("validate agent_id: must not be nil")
	}

	if c.BytesPerSecond <= 0 {
		return xerrors.Errorf("validate bytes_per_second: must be greater than zero")
	}

	if c.Duration <= 0 {
		return xerrors.Errorf("validate duration: must be greater than zero")
	}

	if c.TicksPerSecond <= 0 {
		return xerrors.Errorf("validate ticks_per_second: must be greater than zero")
	}
	return nil
}
