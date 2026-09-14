package chat

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// Config describes a single chat runner within a scaletest invocation.
type Config struct {
	// OrganizationID is the organization that owns the target workspace.
	OrganizationID uuid.UUID `json:"organization_id"`

	// WorkspaceID is the pre-existing workspace to use for this chat run.
	// When empty, the chat runs without workspace context.
	WorkspaceID uuid.UUID `json:"workspace_id"`

	// Prompt is the text content sent on every turn.
	Prompt string `json:"prompt"`

	// ModelConfigID is the scaletest mock LLM model config.
	ModelConfigID uuid.UUID `json:"model_config_id"`

	// Turns is the total number of user to assistant exchanges per chat.
	// Must be at least 1.
	Turns int `json:"turns"`

	// TurnStartDelay is the shared delay between every runner completing
	// its initial turn and the release of the follow-up turns. Set
	// to 0 to send all turns without an inter-phase pause.
	TurnStartDelay time.Duration `json:"turn_start_delay"`

	// TurnStartReadyWaitGroup coordinates the gap between the initial turn
	// finishing and the follow-up turns. Each runner signals exactly
	// once after its first turn reaches a terminal status, or when it
	// knows it will never reach that point.
	TurnStartReadyWaitGroup *sync.WaitGroup `json:"-"`

	// StartTurnsChan blocks follow-up turns until the CLI layer releases them.
	StartTurnsChan chan struct{} `json:"-"`

	Metrics *Metrics `json:"-"`
}

func (c Config) Validate() error {
	if c.OrganizationID == uuid.Nil {
		return xerrors.Errorf("validate organization_id: must not be empty")
	}
	if c.Prompt == "" {
		return xerrors.Errorf("validate prompt: must not be empty")
	}
	if c.ModelConfigID == uuid.Nil {
		return xerrors.Errorf("validate model_config_id: must not be empty")
	}
	if c.Turns < 1 {
		return xerrors.Errorf("validate turns: must be at least 1")
	}
	if c.TurnStartDelay < 0 {
		return xerrors.Errorf("validate turn_start_delay: must not be negative")
	}
	if c.TurnStartDelay > 0 && c.Turns > 1 {
		if c.TurnStartReadyWaitGroup == nil {
			return xerrors.Errorf("validate turn_start_ready_wait_group: must not be nil when turn start delay is enabled for more than one turn")
		}
		if c.StartTurnsChan == nil {
			return xerrors.Errorf("validate start_turns_chan: must not be nil when turn start delay is enabled for more than one turn")
		}
	}
	if c.Metrics == nil {
		return xerrors.Errorf("validate metrics: must not be nil")
	}

	return nil
}
