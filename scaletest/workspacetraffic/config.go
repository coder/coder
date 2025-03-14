package workspacetraffic
import (
	"fmt"
	"errors"
	"time"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/codersdk"
)
type Config struct {
	// AgentID is the workspace agent ID to which to connect.
	AgentID uuid.UUID `json:"agent_id"`
	// BytesPerTick is the number of bytes to send to the agent per tick.
	BytesPerTick int64 `json:"bytes_per_tick"`
	// Duration is the total duration for which to send traffic to the agent.
	Duration time.Duration `json:"duration"`
	// TickInterval specifies the interval between ticks (that is, attempts to
	// send data to workspace agents).
	TickInterval time.Duration `json:"tick_interval"`
	ReadMetrics  ConnMetrics `json:"-"`
	WriteMetrics ConnMetrics `json:"-"`
	SSH bool `json:"ssh"`
	// Echo controls whether the agent should echo the data it receives.
	// If false, the agent will discard the data. Note that setting this
	// to true will double the amount of data read from the agent for
	// PTYs (e.g. reconnecting pty or SSH connections that request PTY).
	Echo bool `json:"echo"`
	App AppConfig `json:"app"`
	WebClient *codersdk.Client
}
func (c Config) Validate() error {
	if c.AgentID == uuid.Nil {
		return fmt.Errorf("validate agent_id: must not be nil")
	}
	if c.BytesPerTick <= 0 {
		return fmt.Errorf("validate bytes_per_tick: must be greater than zero")
	}
	if c.Duration <= 0 {
		return fmt.Errorf("validate duration: must be greater than zero")
	}
	if c.TickInterval <= 0 {
		return fmt.Errorf("validate tick_interval: must be greater than zero")
	}
	if c.SSH && c.App.Name != "" {
		return fmt.Errorf("validate ssh: must be false when app is used")
	}
	return nil
}
type AppConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
