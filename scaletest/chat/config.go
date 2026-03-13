package chat

import (
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type Config struct {
	// WorkspaceID is the pre-existing workspace to create chats against.
	WorkspaceID uuid.UUID `json:"workspace_id"`

	// Prompt is the text content for the initial chat message.
	Prompt string `json:"prompt"`

	// ModelConfigID optionally selects a specific model config.
	// When nil the server uses its deployment default.
	ModelConfigID *uuid.UUID `json:"model_config_id,omitempty"`

	// Turns is the total number of user→assistant exchanges per chat.
	// Must be at least 1.
	Turns int `json:"turns"`

	// FollowUpPrompt is the text content for turns 2..N.
	FollowUpPrompt string `json:"follow_up_prompt"`

	// ReadyWaitGroup is used to coordinate thundering-herd fanout from the CLI
	// layer. Each runner calls Done() once it is ready to start.
	ReadyWaitGroup *sync.WaitGroup `json:"-"`

	// StartChan blocks runners before creating chats. The CLI layer closes it
	// once all runners are ready.
	StartChan chan struct{} `json:"-"`

	Metrics           *Metrics `json:"-"`
	MetricLabelValues []string `json:"metric_label_values"`
}

func (c *Config) Validate() error {
	if c.WorkspaceID == uuid.Nil {
		return xerrors.Errorf("validate workspace_id: must not be nil")
	}

	if c.Prompt == "" {
		return xerrors.Errorf("validate prompt: must not be empty")
	}

	if c.Turns < 1 {
		return xerrors.Errorf("validate turns: must be at least 1")
	}

	if c.Turns > 1 && c.FollowUpPrompt == "" {
		return xerrors.Errorf("validate follow_up_prompt: must not be empty when turns > 1")
	}

	if c.ReadyWaitGroup == nil {
		return xerrors.Errorf("validate ready_wait_group: must not be nil")
	}

	if c.StartChan == nil {
		return xerrors.Errorf("validate start_chan: must not be nil")
	}

	if c.Metrics == nil {
		return xerrors.Errorf("validate metrics: must not be nil")
	}

	return nil
}
