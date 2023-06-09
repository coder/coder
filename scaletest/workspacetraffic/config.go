package workspacetraffic

import (
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"
)

type Config struct {
	// AgentID is the workspace agent ID to which to connect.
	AgentID uuid.UUID `json:"agent_id"`

	// AgentName is the name of the agent. Used for metrics.
	AgentName string `json:"agent_name"`

	// WorkspaceName is the name of the workspace. Used for metrics.
	WorkspaceName string `json:"workspace_name"`

	// WorkspaceOwner is the owner of the workspace. Used for metrics.
	WorkspaceOwner string `json:"workspace_owner"`

	// BytesPerTick is the number of bytes to send to the agent per tick.
	BytesPerTick int64 `json:"bytes_per_tick"`

	// Duration is the total duration for which to send traffic to the agent.
	Duration time.Duration `json:"duration"`

	// TickInterval specifies the interval between ticks (that is, attempts to
	// send data to workspace agents).
	TickInterval time.Duration `json:"tick_interval"`

	// Registry is a prometheus.Registerer for logging metrics
	Registry prometheus.Registerer
}

func (c Config) Validate() error {
	if c.AgentID == uuid.Nil {
		return xerrors.Errorf("validate agent_id: must not be nil")
	}

	if c.BytesPerTick <= 0 {
		return xerrors.Errorf("validate bytes_per_tick: must be greater than zero")
	}

	if c.Duration <= 0 {
		return xerrors.Errorf("validate duration: must be greater than zero")
	}

	if c.TickInterval <= 0 {
		return xerrors.Errorf("validate tick_interval: must be greater than zero")
	}

	return nil
}
