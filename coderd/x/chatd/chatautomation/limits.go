package chatautomation

import (
	"time"

	"github.com/coder/coder/v2/codersdk"
)

// Limits are the deployment settings that Create enforces.
type Limits struct {
	// MaxPerOwner is the number of automations that one user can own across
	// all organizations.
	MaxPerOwner int64
	// MinInterval is the shortest time between two runs of a schedule.
	MinInterval time.Duration
	// MaxPromptBytes is the longest prompt, in bytes.
	MaxPromptBytes int
}

// LimitsFromConfig reads the limits from the deployment chat configuration.
func LimitsFromConfig(cfg codersdk.ChatConfig) Limits {
	return Limits{
		MaxPerOwner:    cfg.MaxAutomationsPerOwner.Value(),
		MinInterval:    cfg.MinAutomationScheduleInterval.Value(),
		MaxPromptBytes: int(cfg.MaxPromptBytes.Value()),
	}
}
